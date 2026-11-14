//go:build windows

package rdp

import "unsafe"

// The objects the desktop's control calls back into, and the tables of
// their methods: the client site the control is embedded through, the site
// it is activated in place in, the frame that site gives it and the
// container that site belongs to. Each table is the interface as oleidl.h
// declares it, starting with the three methods of IUnknown, in the order
// of the interface and of the ones it extends: IOleInPlaceSite keeps the
// OnUIActivate and Scroll of the first OLE, which the control counts on.

// clientSite is the control's IOleClientSite.
type clientSite struct{ comHead }

// inPlaceSite is its IOleInPlaceSite, the site it is activated in place
// in: the same object as the client site, in another interface.
type inPlaceSite struct{ comHead }

// frame is its IOleInPlaceFrame, which has no menus to give it and no
// border space.
type frame struct{ comHead }

// container is its IOleContainer, which holds the control alone and has no
// names to parse.
type container struct{ comHead }

var (
	clientSiteVtbl = vtbl(
		cb(comQueryInterface), cb(comAddRef), cb(comRelease),
		cb(siteSaveObject), cb(siteGetMoniker), cb(siteGetContainer),
		cb(siteShowObject), cb(siteOnShowWindow), cb(siteRequestNewObjectLayout),
	)
	inPlaceSiteVtbl = vtbl(
		cb(comQueryInterface), cb(comAddRef), cb(comRelease),
		cb(siteGetWindow), cb(siteContextSensitiveHelp), cb(siteCanInPlaceActivate),
		cb(siteOnInPlaceActivate), cb(siteOnUIActivate), cb(siteGetWindowContext),
		cb(siteScroll), cb(siteOnUIDeactivate), cb(siteOnInPlaceDeactivate),
		cb(siteDiscardUndoState), cb(siteDeactivateAndUndo), cb(siteOnPosRectChange),
	)
	frameVtbl = vtbl(
		cb(comQueryInterface), cb(comAddRef), cb(comRelease),
		cb(frameGetWindow), cb(siteContextSensitiveHelp), cb(frameGetBorder),
		cb(frameRequestBorderSpace), cb(frameSetBorderSpace), cb(frameSetActiveObject),
		cb(frameInsertMenus), cb(frameSetMenu), cb(frameRemoveMenus),
		cb(frameSetStatusText), cb(frameEnableModeless), cb(frameTranslateAccelerator),
	)
	containerVtbl = vtbl(
		cb(comQueryInterface), cb(comAddRef), cb(comRelease),
		cb(containerParseDisplayName), cb(containerEnumObjects), cb(containerLockContainer),
	)
)

// newSite makes the objects of the desktop, which it keeps: the client
// site, the in place site, the frame and the container.
func (e *Embedded) newSite() {
	e.site = &clientSite{comHead{vtbl: clientSiteVtbl, refs: 1, host: e, query: e.queryClientSite}}
	e.place = &inPlaceSite{comHead{vtbl: inPlaceSiteVtbl, refs: 1, host: e, query: e.queryInPlaceSite}}
	e.frame = &frame{comHead{vtbl: frameVtbl, refs: 1, host: e, query: e.queryFrame}}
	e.container = &container{comHead{vtbl: containerVtbl, refs: 1, host: e, query: e.queryContainer}}
}

// The interfaces the site answers for, and those of the frame and the
// container: each is itself, the interfaces it extends, and IUnknown.

func (e *Embedded) queryClientSite(g *guid) unsafe.Pointer {
	switch *g {
	case iidUnknown, iidIOleClientSite:
		return unsafe.Pointer(e.site)
	case iidIOleInPlaceSite:
		return unsafe.Pointer(e.place)
	}
	return nil
}

func (e *Embedded) queryInPlaceSite(g *guid) unsafe.Pointer {
	switch *g {
	case iidUnknown, iidIOleInPlaceSite, iidIOleWindow:
		return unsafe.Pointer(e.place)
	case iidIOleInPlaceFrame, iidIOleInPlaceUIWindow:
		return unsafe.Pointer(e.frame)
	}
	return nil
}

func (e *Embedded) queryFrame(g *guid) unsafe.Pointer {
	switch *g {
	case iidUnknown, iidIOleInPlaceFrame, iidIOleInPlaceUIWindow, iidIOleWindow:
		return unsafe.Pointer(e.frame)
	}
	return nil
}

func (e *Embedded) queryContainer(g *guid) unsafe.Pointer {
	switch *g {
	case iidUnknown, iidIOleContainer, iidIParseDisplayName:
		return unsafe.Pointer(e.container)
	}
	return nil
}

// IOleClientSite.

func siteSaveObject(this uintptr) uintptr { return sOK }

func siteGetMoniker(this, assign, which, ppmk uintptr) uintptr {
	if ppmk != 0 {
		*(*uintptr)(native(ppmk)) = 0
	}
	return eNotImpl
}

func siteGetContainer(this, ppv uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	e := headOf(this).host
	*(*uintptr)(native(ppv)) = uintptr(unsafe.Pointer(e.container))
	e.container.refs++
	return sOK
}

func siteShowObject(this uintptr) uintptr { return sOK }

func siteOnShowWindow(this, show uintptr) uintptr { return sOK }

func siteRequestNewObjectLayout(this uintptr) uintptr { return eNotImpl }

// IOleInPlaceSite.

func siteGetWindow(this, phwnd uintptr) uintptr {
	if phwnd == 0 {
		return ePointer
	}
	*(*uintptr)(native(phwnd)) = headOf(this).host.hwnd
	return sOK
}

func siteContextSensitiveHelp(this, enter uintptr) uintptr { return eNotImpl }

func siteCanInPlaceActivate(this uintptr) uintptr { return sOK }

func siteOnInPlaceActivate(this uintptr) uintptr {
	headOf(this).host.inside = true
	return sOK
}

// siteOnUIActivate is the control taking the window's menus, which this
// container has none of: the window is the app's.
func siteOnUIActivate(this uintptr) uintptr { return sOK }

// siteGetWindowContext is what the control takes the place it is drawn in
// from: the frame, and the client box of the host window.
func siteGetWindowContext(this, ppFrame, ppDoc, pos, clip, info uintptr) uintptr {
	e := headOf(this).host
	if ppFrame != 0 {
		*(*uintptr)(native(ppFrame)) = uintptr(unsafe.Pointer(e.frame))
		e.frame.refs++
	}
	if ppDoc != 0 {
		*(*uintptr)(native(ppDoc)) = 0
	}
	box := e.clientRect()
	if pos != 0 {
		*(*winRect)(native(pos)) = box
	}
	if clip != 0 {
		*(*winRect)(native(clip)) = box
	}
	if info != 0 {
		*(*inPlaceFrameInfo)(native(info)) = inPlaceFrameInfo{
			cb:        uint32(unsafe.Sizeof(inPlaceFrameInfo{})),
			hwndFrame: e.hwnd,
		}
	}
	return sOK
}

// siteScroll scrolls a box of the window's pixels, which the control fills
// alone: there is nothing of the app's under it to scroll.
func siteScroll(this, dx, dy, prcScroll uintptr) uintptr { return sOK }

func siteOnUIDeactivate(this, undoable uintptr) uintptr {
	headOf(this).host.inside = false
	return sOK
}

func siteOnInPlaceDeactivate(this uintptr) uintptr {
	headOf(this).host.inside = false
	return sOK
}

func siteDiscardUndoState(this uintptr) uintptr { return eNotImpl }

func siteDeactivateAndUndo(this uintptr) uintptr { return eNotImpl }

// siteOnPosRectChange is the control asking for a place: it is given the
// box it asks for, as the container is the one that decides.
func siteOnPosRectChange(this, prc uintptr) uintptr {
	e := headOf(this).host
	if prc == 0 {
		return ePointer
	}
	e.setObjectRects(*(*winRect)(native(prc)), e.clientRect())
	return sOK
}

// IOleInPlaceUIWindow and IOleInPlaceFrame: no menus and no border space.

func frameGetWindow(this, phwnd uintptr) uintptr {
	if phwnd == 0 {
		return ePointer
	}
	*(*uintptr)(native(phwnd)) = headOf(this).host.hwnd
	return sOK
}

func frameGetBorder(this, border uintptr) uintptr { return eNotImpl }

func frameRequestBorderSpace(this, width uintptr) uintptr { return eNotImpl }

func frameSetBorderSpace(this, width uintptr) uintptr { return eNotImpl }

func frameSetActiveObject(this, obj, name uintptr) uintptr { return sOK }

func frameInsertMenus(this, menu, widths uintptr) uintptr { return eNotImpl }

func frameSetMenu(this, menu, shared, hwnd uintptr) uintptr { return eNotImpl }

func frameRemoveMenus(this, menu uintptr) uintptr { return eNotImpl }

func frameSetStatusText(this, text uintptr) uintptr { return sOK }

func frameEnableModeless(this, enable uintptr) uintptr { return sOK }

func frameTranslateAccelerator(this, msg, id uintptr) uintptr { return sFalse }

// IOleContainer.

func containerParseDisplayName(this, bindCtx, name, eaten, moniker uintptr) uintptr {
	if moniker != 0 {
		*(*uintptr)(native(moniker)) = 0
	}
	return eNotImpl
}

func containerEnumObjects(this, flags, enum uintptr) uintptr {
	if enum != 0 {
		*(*uintptr)(native(enum)) = 0
	}
	return eNotImpl
}

func containerLockContainer(this, lock uintptr) uintptr { return sOK }
