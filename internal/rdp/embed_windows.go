//go:build windows

package rdp

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// EmbedSupported tells whether a remote desktop can be drawn in a window of
// the app's: Windows brings the control that draws mstsc's desktop, an
// ActiveX control the app activates in place in a window of its own, which
// it places where the tab draws the desktop and hides while another tab
// shows. See Embed.
func EmbedSupported() bool { return true }

var (
	modOle32    = windows.NewLazySystemDLL("ole32.dll")
	modOleAut32 = windows.NewLazySystemDLL("oleaut32.dll")
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procOleInitialize    = modOle32.NewProc("OleInitialize")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procSysAllocString   = modOleAut32.NewProc("SysAllocString")
	procSysFreeString    = modOleAut32.NewProc("SysFreeString")

	procRegisterClassExW    = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW     = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow       = modUser32.NewProc("DestroyWindow")
	procDefWindowProcW      = modUser32.NewProc("DefWindowProcW")
	procShowWindow          = modUser32.NewProc("ShowWindow")
	procSetWindowPos        = modUser32.NewProc("SetWindowPos")
	procGetClientRect       = modUser32.NewProc("GetClientRect")
	procSetFocus            = modUser32.NewProc("SetFocus")
	procGetForegroundWindow = modUser32.NewProc("GetForegroundWindow")
	procGetAncestor         = modUser32.NewProc("GetAncestor")
	procEnumChildWindows    = modUser32.NewProc("EnumChildWindows")
	procGetDpiForWindow     = modUser32.NewProc("GetDpiForWindow")
	procClientToScreen      = modUser32.NewProc("ClientToScreen")
	procIsIconic            = modUser32.NewProc("IsIconic")
	procIsWindowVisible     = modUser32.NewProc("IsWindowVisible")
	procIsWindowEnabled     = modUser32.NewProc("IsWindowEnabled")
	procSetTimer            = modUser32.NewProc("SetTimer")
	procKillTimer           = modUser32.NewProc("KillTimer")

	procCreateSolidBrush = modGdi32.NewProc("CreateSolidBrush")
	procGetModuleHandleW = modKernel32.NewProc("GetModuleHandleW")
)

// What the control is made by and activated through.
const (
	clsctxInProcServer = 1

	wsPopup = 0x80000000

	wsExToolWindow = 0x00000080

	swHide           = 0
	swShowNoActivate = 4

	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001

	// wmSize, wmSetFocus and wmTimer, as the host window's procedure takes
	// them.
	wmSize     = 0x0005
	wmSetFocus = 0x0007
	wmTimer    = 0x0113

	oleCloseNoSave = 1

	// inPlaceActivate is OLEIVERB_INPLACEACTIVATE (-5), the verb that has
	// the control draw itself in the site's window.
	inPlaceActivate = 0xfffffffb

	gaRoot = 2

	// hostWidth and hostHeight are the size the host window opens at,
	// before the app places it in the tab.
	hostWidth, hostHeight = 800, 600

	// fitDelay is how long the app's resizing of the window is left to
	// settle before the session is reconnected at its new size, and
	// resizeTimeout how long that reconnect has before a connection that
	// stays away counts as gone.
	fitDelay      = 300 * time.Millisecond
	resizeTimeout = 15 * time.Second
)

// hostClass is the class of the window the control draws in: a window of its
// own, over the app's, which the control fills.
var hostClass = windows.StringToUTF16Ptr("FinallyShellRemoteDesktop")

// winRect is a RECT: a box in a window's pixels.
type winRect struct{ left, top, right, bottom int32 }

// point is POINT: a place in the screen's pixels.
type point struct{ x, y int32 }

func (r winRect) width() int32  { return r.right - r.left }
func (r winRect) height() int32 { return r.bottom - r.top }

// inPlaceFrameInfo is OLEINPLACEFRAMEINFO.
type inPlaceFrameInfo struct {
	cb           uint32
	mdiapp       int32
	hwndFrame    uintptr
	accel        uintptr
	accelEntries uint32
}

// wndClassEx is WNDCLASSEXW, the class of the host window.
type wndClassEx struct {
	size          uint32
	style         uint32
	wndProc       uintptr
	clsExtra      int32
	wndExtra      int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	menuName      *uint16
	className     *uint16
	hIconSm       uintptr
}

// Embedded is a remote desktop drawn in a window of the app's, by Windows'
// Remote Desktop control: the host window the app places, the control
// activated in place inside it, and where the connection is (see Desktop).
// Its methods belong to the thread Embed ran on, which the control lives
// in.
type Embedded struct {
	parent  uintptr // the app's window, which the host window belongs to
	hwnd    uintptr // the host window, 0 until it is made
	ctrl    uintptr // the control's IOleObject
	inPlace uintptr // its IOleInPlaceObject, once activated in place
	d       disp    // its properties
	secret  uintptr // its IMsTscNonScriptable, which the password goes through

	site      *clientSite
	place     *inPlaceSite
	frame     *frame
	container *container
	// events is the sink the control tells its events to, at point its
	// connection point, under cookie; the ids of event names are those
	// the control's type information gives them (events_windows.go).
	events *sink
	point  uintptr
	cookie uint32
	diid   guid     // the events' interface, as the sink is asked for it
	ids    [5]int32 // connecting, connected, logged in, disconnected, failed
	// told tells whether the control tells its events: one that does not
	// is followed by polling its state.
	told bool

	cfg   Config
	at    Rect    // where the tab draws the desktop, in the app's DIPs
	pix   winRect // where the host window is, in the screen's pixels
	shown bool    // the host window shows
	// hidden tells whether the host window is out of sight because the
	// app's window is, which the timer follows (follow).
	hidden bool
	// want is the size the window has now, due when the session is
	// reconnected at it (fit), and resizing tells whether that reconnect
	// is under way, since when.
	want     [2]int
	due      time.Time
	resizing bool
	resizeAt time.Time
	fitting  int // the step of a resize: 1 dropped, 2 to make again, 3 made
	// inside tells whether the control is activated in place: it draws the
	// desktop from then on.
	inside    bool
	started   bool  // Connect has been called
	connected bool  // the desktop shows
	ended     bool  // the desktop went away
	reason    int32 // why it went away, as the control counts it
	err       error
	closed    bool
}

// The ids of the event names in ids.
const (
	idConnecting = iota
	idConnected
	idLoggedIn
	idDisconnected
	idFailed
)

var (
	oleOnce sync.Once
	oleErr  error

	hostOnce sync.Once
	hostErr  error

	// hosts are the desktops by the window the control draws in, as the
	// window's procedure finds them.
	hostsMu sync.Mutex
	hosts   = map[uintptr]*Embedded{}
)

// Embed opens c in a remote desktop control inside parent, a window of the
// app's, and returns the desktop: it draws in a window of parent's from
// where Place puts it, and connects there, the first time it is placed.
// Nothing is drawn where the system has no control (see ErrNoEmbed); the
// call belongs to the thread of parent, which the control lives in.
func Embed(parent uintptr, c Config) (*Embedded, error) {
	if parent == 0 {
		return nil, errors.New("no window to draw the desktop in")
	}
	if err := oleInit(); err != nil {
		return nil, err
	}
	if err := registerHost(); err != nil {
		return nil, err
	}
	e := &Embedded{parent: parent, cfg: c}
	e.newSite()
	hwnd, err := e.makeHost()
	if err != nil {
		return nil, err
	}
	e.hwnd = hwnd
	hostsMu.Lock()
	hosts[hwnd] = e
	hostsMu.Unlock()
	// The app moves, minimizes and hides its window without building a
	// frame of the tab: a timer has the host window follow it (follow).
	procSetTimer.Call(hwnd, 1, 100, 0)
	if err := e.makeControl(); err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

// oleInit puts this thread in its apartment, as OLE wants it before any
// control is made: on the thread of the window, once.
func oleInit() error {
	oleOnce.Do(func() {
		if r, _, _ := procOleInitialize.Call(0); failed(r) {
			oleErr = hresult("starting OLE", r)
		}
	})
	return oleErr
}

// registerHost registers the class of the window the control draws in,
// once: a window that shows black until the control draws over it.
func registerHost() error {
	hostOnce.Do(func() {
		brush, _, _ := procCreateSolidBrush.Call(0) // black
		hinst, _, _ := procGetModuleHandleW.Call(0)
		wc := wndClassEx{
			size:          uint32(unsafe.Sizeof(wndClassEx{})),
			style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
			wndProc:       cb(hostProc),
			hInstance:     hinst,
			hbrBackground: brush,
			className:     hostClass,
		}
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			hostErr = fmt.Errorf("the class of the desktop's window: %w", err)
		}
	})
	return hostErr
}

// hostProc is the procedure of the host window: it follows its size, and
// hands the keyboard to the control when it takes the focus.
func hostProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	hostsMu.Lock()
	e := hosts[hwnd]
	hostsMu.Unlock()
	if e != nil {
		switch msg {
		case wmSize:
			e.resize()
			return 0
		case wmSetFocus:
			e.focusControl()
			return 0
		case wmTimer:
			e.follow()
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

// makeHost makes the window the control draws in: a window of its own,
// owned by the app's, which the app places over the tab. It is not a child
// of the app's window, which draws its frames in a swap chain that Windows
// puts over its children: a desktop drawn in one there would be out of
// sight. It opens at a size of its own, without showing, until the app
// places it.
func (e *Embedded) makeHost() (uintptr, error) {
	hinst, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, err := procCreateWindowExW.Call(wsExToolWindow,
		uintptr(unsafe.Pointer(hostClass)), 0,
		wsPopup,
		0, 0, hostWidth, hostHeight,
		e.parent, 0, hinst, 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("the window of the desktop: %w", err)
	}
	return hwnd, nil
}

// makeControl makes Windows' remote desktop control, hands it the site it
// is embedded through, the connection's settings and the password, and has
// it draw itself in the host window: it connects when the app places it.
func (e *Embedded) makeControl() error {
	ctrl, err := newControl()
	if err != nil {
		return err
	}
	e.ctrl = ctrl
	if hr := comCall(ctrl, 3, uintptr(unsafe.Pointer(e.site))); failed(hr) { // SetClientSite
		return hresult("the site of the desktop", hr)
	}
	// A control is initialized before it is used, as from a stream no
	// page has: one that does not want it says so.
	var stream uintptr
	if hr := comCall(ctrl, 0, uintptr(unsafe.Pointer(&iidIPersistStreamInit)), uintptr(unsafe.Pointer(&stream))); !failed(hr) {
		comCall(stream, 8) // InitNew
		release(stream)
	}
	var d uintptr
	if hr := comCall(ctrl, 0, uintptr(unsafe.Pointer(&iidIDispatch)), uintptr(unsafe.Pointer(&d))); failed(hr) {
		return hresult("the settings of the desktop", hr)
	}
	e.d = disp{p: d}
	app := bstr("Finallyshell")
	obj := bstr(e.cfg.Address())
	comCall(ctrl, 5, app, obj) // SetHostNames
	freeBstr(app)
	freeBstr(obj)
	if err := e.d.remote(e.cfg); err != nil {
		return err
	}
	e.subscribe()
	var secret uintptr
	if hr := comCall(ctrl, 0, uintptr(unsafe.Pointer(&iidIMsTscNonScriptable)), uintptr(unsafe.Pointer(&secret))); !failed(hr) {
		e.secret = secret
		if e.cfg.Password != "" {
			p := bstr(e.cfg.Password)
			comCall(secret, 3, p) // put_ClearTextPassword
			freeBstr(p)
		}
	}
	box := winRect{right: hostWidth, bottom: hostHeight}
	hr := comCall(ctrl, 11, inPlaceActivate, 0, uintptr(unsafe.Pointer(e.site)), 0, e.hwnd, uintptr(unsafe.Pointer(&box)))
	if failed(hr) {
		return hresult("drawing the desktop", hr)
	}
	var inside uintptr
	if hr := comCall(ctrl, 0, uintptr(unsafe.Pointer(&iidIOleInPlaceObject)), uintptr(unsafe.Pointer(&inside))); failed(hr) {
		return hresult("the place of the desktop", hr)
	}
	e.inPlace = inside
	e.inside = true
	e.setObjectRects(box, box)
	return nil
}

// newControl makes the control of the first of its classes this computer
// has: none is ErrNoEmbed.
func newControl() (uintptr, error) {
	var last error
	for _, clsid := range clsidRDP {
		var ctrl uintptr
		r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, clsctxInProcServer,
			uintptr(unsafe.Pointer(&iidIOleObject)), uintptr(unsafe.Pointer(&ctrl)))
		if !failed(r) && ctrl != 0 {
			return ctrl, nil
		}
		last = hresult("the remote desktop control", r)
	}
	return 0, fmt.Errorf("%w: %v", ErrNoEmbed, last)
}

// Place shows the desktop at r and moves it there, as the app draws its
// tab every frame: the first place is where it connects.
func (e *Embedded) Place(r Rect) {
	if e.closed || e.hwnd == 0 {
		return
	}
	e.at = r
	box := e.screenRect(r)
	if box.width() <= 0 || box.height() <= 0 {
		return
	}
	if !e.shown {
		e.showAt(box)
	}
	if box != e.pix {
		e.move(box)
	}
	if !e.started {
		e.started = true
		e.connect(box)
		e.focus()
	}
}

// screenRect is r, a box of the app's window in DIPs, in the screen's
// pixels, where the host window goes.
func (e *Embedded) screenRect(r Rect) winRect {
	s := dpiScale(e.parent)
	var origin point
	if e.parent != 0 {
		procClientToScreen.Call(e.parent, uintptr(unsafe.Pointer(&origin)))
	}
	at := func(v float32) int32 { return int32(math.Round(float64(v) * s)) }
	x, y := origin.x+at(r.X), origin.y+at(r.Y)
	return winRect{left: x, top: y, right: x + at(r.W), bottom: y + at(r.H)}
}

// showAt shows the host window at box, over the app's window in the order
// of the windows, without taking the focus the app has.
func (e *Embedded) showAt(box winRect) {
	e.pix, e.shown = box, true
	procSetWindowPos.Call(e.hwnd, e.parent, uintptr(int(box.left)), uintptr(int(box.top)),
		uintptr(box.width()), uintptr(box.height()), swpNoActivate)
	procShowWindow.Call(e.hwnd, swShowNoActivate)
	e.resize()
}

// move puts the host window at box.
func (e *Embedded) move(box winRect) {
	e.pix = box
	procSetWindowPos.Call(e.hwnd, 0, uintptr(int(box.left)), uintptr(int(box.top)),
		uintptr(box.width()), uintptr(box.height()), swpNoZOrder|swpNoActivate)
	e.resize()
}

// follow keeps the host window where the app's is: the app moves, resizes,
// shrinks to its taskbar button or hides to the tray while the desktop is
// shown, which the frames of the tab do not tell (see the timer of Embed).
func (e *Embedded) follow() {
	if e.closed || e.hwnd == 0 || !e.shown {
		return
	}
	if e.hidden {
		if !e.windowShows() {
			return
		}
		e.hidden = false
		e.showAt(e.pix)
		return
	}
	if !e.windowShows() {
		e.hidden = true
		procShowWindow.Call(e.hwnd, swHide)
		return
	}
	if box := e.screenRect(e.at); box != e.pix {
		e.move(box)
	}
	if (!e.due.IsZero() && time.Now().After(e.due)) || e.fitting == 2 {
		e.fitStep()
	}
	if e.resizing && time.Since(e.resizeAt) > resizeTimeout {
		// A connection that does not come back at the new size is gone.
		e.resizing, e.fitting = false, 0
		if e.err == nil {
			e.err = ErrEnded
		}
	}
}

// windowShows tells whether the app's window is on the screen: not
// minimized, not hidden behind the tray, and not disabled by a dialog of
// the app's, which the desktop would cover otherwise.
func (e *Embedded) windowShows() bool {
	if e.parent == 0 {
		return false
	}
	if v, _, _ := procIsWindowVisible.Call(e.parent); v == 0 {
		return false
	}
	if v, _, _ := procIsWindowEnabled.Call(e.parent); v == 0 {
		return false
	}
	i, _, _ := procIsIconic.Call(e.parent)
	return i == 0
}

// dpiScale is how many pixels a DIP of window hwnd is: 1 where the system
// does not say.
func dpiScale(hwnd uintptr) float64 {
	if procGetDpiForWindow.Find() != nil {
		return 1
	}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi < 96 {
		return 1
	}
	return float64(dpi) / 96
}

// connect starts the connection: the desktop is the tab's size, which the
// control scales to, unless the connection names a size of its own.
func (e *Embedded) connect(box winRect) {
	e.want = [2]int{int(box.width()), int(box.height())}
	if e.cfg.Width <= 0 || e.cfg.Height <= 0 {
		e.d.put("DesktopWidth", variantInt(int(box.width())))
		e.d.put("DesktopHeight", variantInt(int(box.height())))
	}
	if err := e.d.call("Connect"); err != nil {
		e.err = err
	}
}

// Hide takes the desktop out of sight while its tab is not shown.
func (e *Embedded) Hide() {
	if e.closed || e.hwnd == 0 || !e.shown {
		return
	}
	e.shown, e.hidden = false, false
	procShowWindow.Call(e.hwnd, swHide)
}

// Show shows the desktop where Place put it.
func (e *Embedded) Show() {
	if e.closed || e.hwnd == 0 || e.shown || e.pix.width() <= 0 {
		return
	}
	e.showAt(e.pix)
}

// State is where the connection is: the control connects in its own time,
// and tells the sink when it does (events_windows.go). A control that tells
// nothing is asked whether it is connected instead.
func (e *Embedded) State() (bool, error) {
	switch {
	case e.closed:
		return false, ErrEnded
	case e.err != nil:
		return false, e.err
	case e.connected:
		return true, nil
	case e.ended:
		// Why the desktop went away is the control's to say, once.
		if e.err == nil {
			e.err = e.endError()
		}
		return false, e.err
	case !e.started || e.told:
		return false, nil
	}
	ok, err := e.d.boolean("Connected")
	if err != nil {
		return false, err
	}
	e.connected = ok
	return ok, nil
}

// Close ends the connection and takes the desktop out of the window, as
// closing mstsc's window does.
func (e *Embedded) Close() {
	if e.closed {
		return
	}
	e.closed = true
	e.unsubscribe()
	if e.d.p != 0 {
		e.d.call("Disconnect")
	}
	if e.ctrl != 0 {
		comCall(e.ctrl, 6, oleCloseNoSave) // IOleObject::Close
	}
	release(e.inPlace)
	release(e.d.p)
	release(e.secret)
	release(e.ctrl)
	e.inPlace, e.d.p, e.secret, e.ctrl = 0, 0, 0, 0
	if e.hwnd != 0 {
		procKillTimer.Call(e.hwnd, 1)
		hostsMu.Lock()
		delete(hosts, e.hwnd)
		hostsMu.Unlock()
		procDestroyWindow.Call(e.hwnd)
		e.hwnd = 0
	}
}

// endError is why the desktop ended: the control's words for the reason it
// gave, or the reason alone.
func (e *Embedded) endError() error {
	return EndError{Reason: e.reason, Text: e.errorDescription()}
}

// errorDescription is the control's words for the end of the connection, ""
// when it has none: GetErrorDescription's, or the text of its disconnected
// page.
func (e *Embedded) errorDescription() string {
	if e.d.p == 0 {
		return ""
	}
	if text := e.d.description(e.reason); text != "" {
		return text
	}
	return e.text("DisconnectedText")
}

// text is the string the property name holds, "" for none.
func (e *Embedded) text(name string) string {
	v, err := e.d.get(name)
	if err != nil || v.vt != vtBstr {
		return ""
	}
	defer freeBstr(v.val)
	if v.val == 0 {
		return ""
	}
	return windows.UTF16PtrToString((*uint16)(native(v.val)))
}

// clientRect is the box of the host window's content, which the control
// fills.
func (e *Embedded) clientRect() winRect {
	var r winRect
	if e.hwnd == 0 {
		return r
	}
	procGetClientRect.Call(e.hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

// resize has the control fill the host window again, and a desktop that is
// connected take the size of the window, as mstsc's does.
func (e *Embedded) resize() {
	box := e.clientRect()
	e.setObjectRects(box, box)
	if e.connected && (e.want[0] != int(box.width()) || e.want[1] != int(box.height())) {
		e.fit(box)
	}
}

// fit notes the size the window has now, which the session is connected at
// once the app stops resizing it: a drag is one reconnect, at its end.
func (e *Embedded) fit(box winRect) {
	w, h := int(box.width()), int(box.height())
	if w < 200 || h < 200 || (e.want[0] == w && e.want[1] == h) {
		return
	}
	e.want, e.due = [2]int{w, h}, time.Now().Add(fitDelay)
}

// fitStep takes the next step of a resize: the connection is dropped, and
// made again at the new size once it is gone (event, in events_windows.go).
func (e *Embedded) fitStep() {
	switch {
	case e.fitting == 0:
		e.due = time.Time{}
		if e.closed || e.d.p == 0 || e.want[0] < 200 || e.want[1] < 200 {
			return
		}
		e.fitting, e.resizing, e.resizeAt = 1, true, time.Now()
		e.d.call("Disconnect")
	case e.fitting == 2:
		e.fitting = 3
		e.d.put("DesktopWidth", variantInt(e.want[0]))
		e.d.put("DesktopHeight", variantInt(e.want[1]))
		if err := e.d.call("Connect"); err != nil {
			e.err = err
		}
	}
}

// setObjectRects is where the control draws: the box in the window, and
// what of it shows.
func (e *Embedded) setObjectRects(pos, clip winRect) {
	if e.inPlace == 0 {
		return
	}
	comCall(e.inPlace, 7, uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&clip)))
}

// focus gives the control the keyboard, as a click on the desktop does,
// when the app's window is the one in front: the connection opened there.
func (e *Embedded) focus() {
	fg, _, _ := procGetForegroundWindow.Call()
	root, _, _ := procGetAncestor.Call(fg, gaRoot)
	if fg != e.parent && root != e.parent {
		return
	}
	e.focusControl()
}

// focusControl gives the keyboard to the control's window, the one it
// draws in.
func (e *Embedded) focusControl() {
	var win uintptr
	if e.inPlace != 0 {
		comCall(e.inPlace, 3, uintptr(unsafe.Pointer(&win))) // IOleInPlaceObject::GetWindow
	}
	if win == 0 {
		win = firstChild(e.hwnd)
	}
	if win != 0 {
		procSetFocus.Call(win)
	}
}

// childFound is the window the enumeration of a window's children stopped
// at: the calls are the window's thread's, one at a time.
var childFound uintptr

var enumChild = syscall.NewCallback(func(child, lparam uintptr) uintptr {
	childFound = child
	return 0 // the first is the one
})

// firstChild is the first window hwnd holds, 0 for none.
func firstChild(hwnd uintptr) uintptr {
	if hwnd == 0 {
		return 0
	}
	childFound = 0
	procEnumChildWindows.Call(hwnd, enumChild, 0)
	return childFound
}
