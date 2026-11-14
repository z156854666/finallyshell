//go:build windows

package rdp

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The events of the control: the connection says there when it connects,
// when it ends and why it failed, which the tab shows. They are told to an
// IDispatch of this package's, the sink, which the control's connection
// point calls, and whose names come from the type information of the class
// the control belongs to.

var (
	iidIProvideClassInfo         = guidOf("b196b283-bab4-101a-b69c-00aa00341d07")
	iidIConnectionPoint          = guidOf("b196b286-bab4-101a-b69c-00aa00341d07")
	iidIConnectionPointContainer = guidOf("b196b284-bab4-101a-b69c-00aa00341d07")
)

// sink is the IDispatch the control tells its events to.
type sink struct{ comHead }

var sinkVtbl = vtbl(
	cb(comQueryInterface), cb(comAddRef), cb(comRelease),
	cb(sinkTypeInfoCount), cb(sinkGetTypeInfo), cb(sinkGetIDsOfNames), cb(sinkInvoke),
)

// querySink answers the interfaces of the sink: the IDispatch, and the
// events' interface itself, which a connection point asks the sink for by
// its identifier before it tells it anything.
func (e *Embedded) querySink(g *guid) unsafe.Pointer {
	switch *g {
	case iidUnknown, iidIDispatch:
		return unsafe.Pointer(e.events)
	}
	if *g == e.diid {
		return unsafe.Pointer(e.events)
	}
	return nil
}

func sinkTypeInfoCount(this, count uintptr) uintptr {
	if count != 0 {
		*(*uint32)(native(count)) = 0
	}
	return sOK
}

func sinkGetTypeInfo(this, index, info uintptr) uintptr { return eNotImpl }

func sinkGetIDsOfNames(this, names, count, lcid, ids uintptr) uintptr { return eNotImpl }

// sinkInvoke is an event of the connection, told by the id its name has in
// the control's type information.
func sinkInvoke(this, id, riid, lcid, flags, params, result, except, argErr uintptr) uintptr {
	e := headOf(this).host
	if result != 0 {
		*(*variant)(native(result)) = variant{}
	}
	e.event(int32(id), (*dispParams)(native(params)))
	return sOK
}

// event follows the connection to what the control says of it.
func (e *Embedded) event(id int32, p *dispParams) {
	switch {
	case id == 0:
	case id == e.ids[idConnected], id == e.ids[idLoggedIn]:
		e.connected, e.resizing, e.fitting = true, false, 0
	case id == e.ids[idDisconnected]:
		if e.fitting == 1 {
			e.fitting = 2 // the connection is gone: make it again, sized
			return
		}
		if e.resizing {
			return // the connection away while it takes the new size
		}
		// The reason is the code OnDisconnected takes, which the control
		// itself words (see endError).
		e.connected, e.ended = false, true
		e.reason = eventInt(p, 0)
	case id == e.ids[idFailed]:
		e.connected, e.ended = false, true
		if desc := eventString(p, 2); desc != "" { // the description is the last argument
			e.err = errors.New(desc)
		}
	}
}

// eventString is the string an event's arguments hold, from the last: the
// one n places in, as DISPPARAMS keeps them the last first.
func eventString(p *dispParams, n int) string {
	if p == nil || p.args == nil || int(p.count) <= n {
		return ""
	}
	v := *(*variant)(unsafe.Add(unsafe.Pointer(p.args), uintptr(n)*unsafe.Sizeof(variant{})))
	if v.vt != vtBstr || v.val == 0 {
		return ""
	}
	return windows.UTF16PtrToString((*uint16)(native(v.val)))
}

// eventInt is the integer an event's arguments hold, from the last: the one
// n places in.
func eventInt(p *dispParams, n int) int32 {
	if p == nil || p.args == nil || int(p.count) <= n {
		return 0
	}
	v := *(*variant)(unsafe.Add(unsafe.Pointer(p.args), uintptr(n)*unsafe.Sizeof(variant{})))
	return *(*int32)(unsafe.Pointer(&v.val))
}

// typeAttr is the start of TYPEATTR, which begins with the GUID of the
// type: that of the interface it is the information of.
type typeAttr struct{ guid guid }

// subscribe has the control tell its events to the sink, and takes the ids
// of their names from the type information of its class. A control without
// that information is followed by polling instead (see State).
func (e *Embedded) subscribe() {
	e.events = &sink{comHead{vtbl: sinkVtbl, refs: 1, host: e, query: e.querySink}}
	events := e.eventsInfo()
	if events == 0 {
		return
	}
	defer release(events)
	for i, name := range []string{"OnConnecting", "OnConnected", "OnLoginComplete", "OnDisconnected", "OnFatalError"} {
		if id, ok := typeInfoID(events, name); ok {
			e.ids[i] = id
		}
	}
	if e.ids[idConnected] == 0 {
		return
	}
	// The interface's GUID is the one its connection point is found by.
	var attr uintptr
	if hr := comCall(events, 3, uintptr(unsafe.Pointer(&attr))); failed(hr) { // GetTypeAttr
		return
	}
	diid := (*typeAttr)(native(attr)).guid
	comCall(events, 19, attr) // ReleaseTypeAttr
	e.diid = diid
	var points uintptr
	if hr := comCall(e.ctrl, 0, uintptr(unsafe.Pointer(&iidIConnectionPointContainer)), uintptr(unsafe.Pointer(&points))); failed(hr) {
		return
	}
	defer release(points)
	var point uintptr
	if hr := comCall(points, 4, uintptr(unsafe.Pointer(&diid)), uintptr(unsafe.Pointer(&point))); failed(hr) { // FindConnectionPoint
		return
	}
	var cookie uint32
	if hr := comCall(point, 5, uintptr(unsafe.Pointer(e.events)), uintptr(unsafe.Pointer(&cookie))); failed(hr) { // Advise
		release(point)
		return
	}
	e.point, e.cookie = point, cookie
	e.told = true
}

// unsubscribe stops the control telling its events.
func (e *Embedded) unsubscribe() {
	if e.point != 0 && e.cookie != 0 {
		comCall(e.point, 6, uintptr(e.cookie)) // Unadvise
	}
	e.cookie = 0
	release(e.point)
	e.point = 0
}

// eventsInfo is the type information of the interface the control tells its
// events to, found among those of its class: 0 for none.
func (e *Embedded) eventsInfo() uintptr {
	var info uintptr
	if hr := comCall(e.ctrl, 0, uintptr(unsafe.Pointer(&iidIProvideClassInfo)), uintptr(unsafe.Pointer(&info))); failed(hr) {
		return 0
	}
	defer release(info)
	var class uintptr
	if hr := comCall(info, 3, uintptr(unsafe.Pointer(&class))); failed(hr) { // GetClassInfo
		return 0
	}
	if class == 0 {
		return 0
	}
	defer release(class)
	for i := 0; ; i++ {
		var href uintptr
		if failed(comCall(class, 8, uintptr(i), uintptr(unsafe.Pointer(&href)))) { // GetRefTypeOfImplType
			return 0
		}
		var ref uintptr
		if failed(comCall(class, 14, href, uintptr(unsafe.Pointer(&ref)))) { // GetRefTypeInfo
			continue
		}
		if _, ok := typeInfoID(ref, "OnConnected"); ok {
			return ref
		}
		release(ref)
	}
}

// typeInfoID is the id type information gives name, and whether it has it.
func typeInfoID(info uintptr, name string) (int32, bool) {
	u, err := windows.UTF16FromString(name)
	if err != nil {
		return 0, false
	}
	namep := &u[0]
	var id int32
	hr := comCall(info, 10, uintptr(unsafe.Pointer(&namep)), 1, uintptr(unsafe.Pointer(&id))) // GetIDsOfNames
	runtime.KeepAlive(u)
	if failed(hr) {
		return 0, false
	}
	return id, true
}
