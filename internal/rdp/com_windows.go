//go:build windows

package rdp

import (
	"fmt"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The COM plumbing of the desktop's control (embed_windows.go): the
// identifiers of the interfaces it is made by and calls back into, its
// properties as its dispinterface names them, and the calls themselves, as
// the vtable of an interface holds them.

// The HRESULTs this package returns and compares.
const (
	sOK          = 0
	sFalse       = 1
	eNotImpl     = 0x80004001
	eNoInterface = 0x80004002
	ePointer     = 0x80004003
	eFail        = 0x80004005
)

// guid is a COM GUID: an interface's or a class's identity, as ole32 takes
// them.
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// guidOf is a GUID written as COM writes it: 8-4-4-4-12 hexadecimal digits.
func guidOf(s string) guid {
	var digits []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			digits = append(digits, s[i])
		}
	}
	if len(digits) != 32 {
		panic("rdp: bad GUID " + s)
	}
	b := func(i int) byte {
		v, err := strconv.ParseUint(string(digits[2*i:2*i+2]), 16, 8)
		if err != nil {
			panic("rdp: bad GUID " + s)
		}
		return byte(v)
	}
	g := guid{
		Data1: uint32(b(0))<<24 | uint32(b(1))<<16 | uint32(b(2))<<8 | uint32(b(3)),
		Data2: uint16(b(4))<<8 | uint16(b(5)),
		Data3: uint16(b(6))<<8 | uint16(b(7)),
	}
	for i := range g.Data4 {
		g.Data4[i] = b(8 + i)
	}
	return g
}

var (
	// The interfaces the control is made by and activated in place
	// through, and those it calls back into.
	iidNull                = guid{}
	iidUnknown             = guidOf("00000000-0000-0000-c000-000000000046")
	iidIDispatch           = guidOf("00020400-0000-0000-c000-000000000046")
	iidIOleObject          = guidOf("00000112-0000-0000-c000-000000000046")
	iidIOleInPlaceObject   = guidOf("00000113-0000-0000-c000-000000000046")
	iidIOleWindow          = guidOf("00000114-0000-0000-c000-000000000046")
	iidIOleInPlaceUIWindow = guidOf("00000115-0000-0000-c000-000000000046")
	iidIOleInPlaceFrame    = guidOf("00000116-0000-0000-c000-000000000046")
	iidIOleClientSite      = guidOf("00000118-0000-0000-c000-000000000046")
	iidIOleInPlaceSite     = guidOf("00000119-0000-0000-c000-000000000046")
	iidIOleContainer       = guidOf("0000011b-0000-0000-c000-000000000046")
	iidIParseDisplayName   = guidOf("0000011c-0000-0000-c000-000000000046")
	iidIPersistStreamInit  = guidOf("7fd52380-4e07-101b-ae2d-08002b2ec713")
	// iidIMsTscNonScriptable is the interface the control's password goes
	// through: its dispinterface leaves it out, as a page may not read it.
	iidIMsTscNonScriptable = guidOf("c1e6743a-41c1-4a74-832a-0dd06c1c7a0e")

	// clsidRDP are the classes of the control, in the order they are
	// tried: the newest NotSafeForScripting one, the control applications
	// host, then the older ones.
	clsidRDP = []guid{
		guidOf("a0c63c30-f08d-4ab4-907c-34905d770c7d"), // MsRdpClient10NotSafeForScripting
		guidOf("8b918b82-7985-4c24-89df-c33ad2bbfbcd"), // MsRdpClient9NotSafeForScripting
		guidOf("54ce37e0-9834-41ae-9896-4dab69dc022b"), // MsTscAxNotSafeForScripting
		guidOf("791fa017-2de3-492e-acc5-53c67a2b94d0"), // MsRdpClient
	}
)

// comCall calls the method index of the interface at p, as its vtable
// holds it, with args, and returns its HRESULT.
func comCall(p uintptr, index int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(native(p))
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(index)*unsafe.Sizeof(uintptr(0))))
	call := make([]uintptr, len(args)+1)
	call[0] = p
	copy(call[1:], args)
	r, _, _ := syscall.SyscallN(fn, call...)
	return r
}

// native is the pointer COM named, which a method takes as an address: its
// this pointer, or a box it is given to fill.
func native(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

// release drops a reference this package holds to the interface at p.
func release(p uintptr) {
	if p != 0 {
		comCall(p, 2)
	}
}

// failed tells whether hr is an HRESULT of failure.
func failed(hr uintptr) bool { return int32(hr) < 0 }

// hresult is a call's failure as an error: which call, and the code the
// system gave it.
func hresult(what string, hr uintptr) error {
	return fmt.Errorf("%s: HRESULT %#08x", what, uint32(hr))
}

// comHead is the head of each object this package hands the control to
// call back into — its client site, the site it is activated in place in,
// its frame and its container: the table of the interface's methods, how
// many references the control holds, the desktop the object belongs to and
// the interfaces it answers for. Every object starts with it, so that the
// this pointer of a method is the object itself.
type comHead struct {
	vtbl  unsafe.Pointer
	refs  int32
	host  *Embedded
	query func(*guid) unsafe.Pointer
}

// headOf is the head of the object a method was called on.
func headOf(this uintptr) *comHead { return (*comHead)(native(this)) }

// comQueryInterface answers the interface riid of the object this.
func comQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	h := headOf(this)
	p := h.query((*guid)(native(riid)))
	if p == nil {
		*(*uintptr)(native(ppv)) = 0
		return eNoInterface
	}
	*(*uintptr)(native(ppv)) = uintptr(p)
	headOf(uintptr(p)).refs++
	return sOK
}

func comAddRef(this uintptr) uintptr {
	h := headOf(this)
	h.refs++
	return uintptr(h.refs)
}

// comRelease counts a reference out. The objects belong to their desktop,
// which takes them down with it, so the last one releases nothing.
func comRelease(this uintptr) uintptr {
	h := headOf(this)
	h.refs--
	if h.refs < 0 {
		h.refs = 0
	}
	return uintptr(h.refs)
}

// cb is fn as COM calls it: a function of the Go runtime's making, which
// takes the method's arguments and returns its HRESULT.
func cb(fn any) uintptr { return syscall.NewCallback(fn) }

// vtbl is an interface's table of methods, which the interface pointer
// this package hands COM points at.
func vtbl(fns ...uintptr) unsafe.Pointer { return unsafe.Pointer(&fns[0]) }

// The VARIANT types of the control's properties, and the flags a call sets.
const (
	vtI2       = 2
	vtI4       = 3
	vtBstr     = 8
	vtDispatch = 9
	vtBool     = 11

	dispatchMethod      = 1
	dispatchPropertyGet = 2
	dispatchPropertyPut = 4

	// dispidPropertyPut is the name a property's value takes when it is
	// set, as IDispatch::Invoke wants it.
	dispidPropertyPut = -3
	localeUserDefault = 0x400
)

// variant is a VARIANT: the type of the value, and the value.
type variant struct {
	vt  uint16
	_   [3]uint16
	val uintptr
	_   uintptr
}

func variantBool(b bool) variant {
	v := variant{vt: vtBool}
	if b {
		v.val = 0xffff // VARIANT_TRUE
	}
	return v
}

func variantInt(n int) variant {
	v := variant{vt: vtI4}
	*(*int32)(unsafe.Pointer(&v.val)) = int32(n)
	return v
}

func variantString(s string) variant { return variant{vt: vtBstr, val: bstr(s)} }

// bstr is s as a BSTR, in memory the system allocates: the one who took
// the VARIANT frees it with freeBstr.
func bstr(s string) uintptr {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0
	}
	p, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(&u[0])))
	runtime.KeepAlive(u)
	return p
}

func freeBstr(p uintptr) {
	if p != 0 {
		procSysFreeString.Call(p)
	}
}

// dispParams is a DISPPARAMS: the arguments of a call, the last argument
// first, and those it names.
type dispParams struct {
	args       *variant
	named      *int32
	count      uint32
	namedCount uint32
}

// disp is the control's dispinterface: its properties and methods by
// name, as mstscax.idl names them. p is the interface pointer of IDispatch
// of the control, 0 before it is made.
type disp struct{ p uintptr }

// id is the ID the interface gives name.
func (d disp) id(name string) (int32, error) {
	if d.p == 0 {
		return 0, ErrNoEmbed
	}
	u, err := windows.UTF16FromString(name)
	if err != nil {
		return 0, err
	}
	namep := &u[0]
	var id int32
	hr := comCall(d.p, 5, uintptr(unsafe.Pointer(&iidNull)), uintptr(unsafe.Pointer(&namep)), 1,
		localeUserDefault, uintptr(unsafe.Pointer(&id)))
	runtime.KeepAlive(u)
	if failed(hr) {
		return 0, hresult("the name of "+name, hr)
	}
	return id, nil
}

// get reads the property name.
func (d disp) get(name string) (variant, error) {
	id, err := d.id(name)
	if err != nil {
		return variant{}, err
	}
	var v variant
	if err := d.invoke("reading "+name, id, dispatchPropertyGet, &dispParams{}, &v); err != nil {
		return variant{}, err
	}
	return v, nil
}

// excepInfo is EXCEPINFO: what a call raised an exception says, its
// description in the words of the system.
type excepInfo struct {
	code         uint16
	reserved     uint16
	_            uint32
	source       uintptr // BSTR
	description  uintptr // BSTR
	helpFile     uintptr // BSTR
	helpContext  uint32
	_            uint32
	reserved2    uintptr
	deferredFill uintptr
	scode        int32
	_            uint32
}

// invoke calls the member id of the control, with params, and puts what it
// answers in result, which is nil for a call that answers nothing.
func (d disp) invoke(what string, id int32, flags uintptr, params *dispParams, result *variant) error {
	var info excepInfo
	var out uintptr
	if result != nil {
		out = uintptr(unsafe.Pointer(result))
	}
	hr := comCall(d.p, 6, uintptr(id), uintptr(unsafe.Pointer(&iidNull)), localeUserDefault, flags,
		uintptr(unsafe.Pointer(params)), out, uintptr(unsafe.Pointer(&info)), 0)
	runtime.KeepAlive(params)
	if !failed(hr) {
		return nil
	}
	// A control that says why says it in the exception.
	msg := ""
	if info.description != 0 {
		msg = windows.UTF16PtrToString((*uint16)(native(info.description)))
	}
	freeBstr(info.source)
	freeBstr(info.description)
	freeBstr(info.helpFile)
	if msg != "" {
		return fmt.Errorf("%s: %s", what, msg)
	}
	return hresult(what, hr)
}

// boolean reads the property name as a flag.
func (d disp) boolean(name string) (bool, error) {
	v, err := d.get(name)
	if err != nil {
		return false, err
	}
	switch v.vt {
	case vtBool, vtI2, vtI4:
		return *(*int32)(unsafe.Pointer(&v.val)) != 0, nil
	}
	if v.vt == vtBstr {
		freeBstr(v.val)
	}
	return false, fmt.Errorf("%s is not a flag", name)
}

// put sets the property name to v: the control takes a copy of a string,
// which is freed here.
func (d disp) put(name string, v variant) error {
	if v.vt == vtBstr {
		defer freeBstr(v.val)
	}
	id, err := d.id(name)
	if err != nil {
		return err
	}
	named := int32(dispidPropertyPut)
	params := dispParams{args: &v, named: &named, count: 1, namedCount: 1}
	return d.invoke("setting "+name, id, dispatchPropertyPut, &params, nil)
}

// call calls the method name, with no arguments.
func (d disp) call(name string) error {
	id, err := d.id(name)
	if err != nil {
		return err
	}
	return d.invoke(name, id, dispatchMethod, &dispParams{}, nil)
}

// description is the control's words for a connection that ended with
// reason, "" when it cannot say: what its GetErrorDescription answers, the
// reason it extends and all.
func (d disp) description(reason int32) string {
	id, err := d.id("GetErrorDescription")
	if err != nil {
		return ""
	}
	var ext int32
	if v, err := d.get("ExtendedDisconnectReason"); err == nil {
		ext = *(*int32)(unsafe.Pointer(&v.val))
	}
	// The arguments come the last first: the extended reason, then the
	// reason the disconnect event gave.
	args := [2]variant{variantInt(int(ext)), variantInt(int(reason))}
	params := dispParams{args: &args[0], count: 2}
	var out variant
	if err := d.invoke("the end of the connection", id, dispatchMethod, &params, &out); err != nil {
		return ""
	}
	if out.vt != vtBstr || out.val == 0 {
		return ""
	}
	defer freeBstr(out.val)
	return windows.UTF16PtrToString((*uint16)(native(out.val)))
}

// remote is the control's properties of a connection: the address, the
// login, the size of the desktop and what it shares, as the connection
// names them.
func (d disp) remote(c Config) error {
	// The control takes the name and the port apart, not as mstsc's
	// command line writes them.
	if err := d.put("Server", variantString(c.Host)); err != nil {
		return err
	}
	if err := d.put("UserName", variantString(c.User)); err != nil {
		return err
	}
	if err := d.put("Domain", variantString(c.Domain)); err != nil {
		return err
	}
	if c.Width > 0 && c.Height > 0 {
		if err := d.put("DesktopWidth", variantInt(c.Width)); err != nil {
			return err
		}
		if err := d.put("DesktopHeight", variantInt(c.Height)); err != nil {
			return err
		}
	}
	if err := d.put("ColorDepth", variantInt(c.depth())); err != nil {
		return err
	}
	// The desktop is the tab's, not the screen's. The control never asks
	// for a password by itself: the app has it, or asked it.
	if err := d.put("FullScreen", variantBool(false)); err != nil {
		return err
	}
	d.put("PromptForCredentials", variantBool(false))
	return d.advanced(c)
}

// advanced sets what the connection shares with the desktop, in the
// control's advanced settings, which an older control may not have: what
// it does not know is left to it.
func (d disp) advanced(c Config) error {
	v, err := d.get("AdvancedSettings")
	if err != nil || v.vt != vtDispatch {
		return nil
	}
	defer release(v.val)
	a := disp{p: v.val}
	if err := a.put("RDPPort", variantInt(c.port())); err != nil {
		return err
	}
	a.put("SmartSizing", variantBool(false))
	a.put("RedirectClipboard", variantBool(c.Clipboard))
	a.put("RedirectDrives", variantBool(c.Drives))
	a.put("RedirectPrinters", variantBool(false))
	a.put("RedirectPorts", variantBool(false))
	a.put("RedirectSmartCards", variantBool(false))
	a.put("RedirectPOSDevices", variantBool(false))
	a.put("AudioRedirectionMode", variantInt(0)) // bring it here
	// A server almost always wants Network Level Authentication, which a
	// hosted control leaves off unless it is told: mstsc turns it on.
	a.put("EnableCredSspSupport", variantBool(true))
	a.put("EnableAutoReconnect", variantBool(true))
	return nil
}
