//go:build windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	pEnumWindows      = user32.NewProc("EnumWindows")
	pGetWindowPID     = user32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisible  = user32.NewProc("IsWindowVisible")
	pGetWindowTextLen = user32.NewProc("GetWindowTextLengthW")
	pSetWindowPos     = user32.NewProc("SetWindowPos")
	pShowWindow       = user32.NewProc("ShowWindow")
	pGetMetrics       = user32.NewProc("GetSystemMetrics")
	pGetClientRect    = user32.NewProc("GetClientRect")
	pGetWindowRect    = user32.NewProc("GetWindowRect")
	pSysParams        = user32.NewProc("SystemParametersInfoW")
	pOpenProcess      = kernel32.NewProc("OpenProcess")
	pCloseHandle      = kernel32.NewProc("CloseHandle")
	pQueryImage       = kernel32.NewProc("QueryFullProcessImageNameW")
)

func procPath(pid uint32) string {
	h, _, _ := pOpenProcess.Call(0x1000, 0, uintptr(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if h == 0 {
		return ""
	}
	defer pCloseHandle.Call(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := pQueryImage.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func sameDir(p, dir string) bool {
	if p == "" || dir == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(filepath.Dir(p)), filepath.Clean(dir)) && strings.EqualFold(filepath.Base(p), "iw5sp.exe")
}

// windows of the game for player 1 (game folder) and player 2 (p2 folder)
func gameWindows() (p1, p2 []uintptr) {
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		vis, _, _ := pIsWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1
		}
		tl, _, _ := pGetWindowTextLen.Call(hwnd)
		if tl == 0 {
			return 1
		}
		var pid uint32
		pGetWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		path := procPath(pid)
		if sameDir(path, cfg.P2Dir) {
			p2 = append(p2, hwnd)
		} else if sameDir(path, cfg.GameDir) {
			p1 = append(p1, hwnd)
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	return
}

func findWindows() (int, int) {
	a, b := gameWindows()
	return len(a), len(b)
}

type rect struct{ l, t, r, b int32 }

var aspects = map[uintptr][2]int{} // original client size of each game window (the picture is rendered at this size)

func clientSize(h uintptr) (int, int) {
	var r rect
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return int(r.r - r.l), int(r.b - r.t)
}

func frameExtra(h uintptr) (int, int) {
	var w, c rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&w)))
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&c)))
	return int(w.r-w.l) - int(c.r-c.l), int(w.b-w.t) - int(c.b-c.t)
}

// place fits the game window into the cell (x,y,w,h). With keep=true the window keeps its original
// picture shape (no stretching) and is centred in the cell; otherwise it fills the cell.
func place(h uintptr, x, y, w, hh int, keep bool) {
	pShowWindow.Call(h, 9) // SW_RESTORE
	if keep {
		if _, ok := aspects[h]; !ok {
			cw, ch := clientSize(h)
			if cw > 0 && ch > 0 {
				aspects[h] = [2]int{cw, ch}
			}
		}
		if a, ok := aspects[h]; ok {
			fx, fy := frameExtra(h)
			availW, availH := w-fx, hh-fy
			cw := availW
			ch := cw * a[1] / a[0]
			if ch > availH {
				ch = availH
				cw = ch * a[0] / a[1]
			}
			ww, wh := cw+fx, ch+fy
			x += (w - ww) / 2
			y += (hh - wh) / 2
			w, hh = ww, wh
		}
	}
	pSetWindowPos.Call(h, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(hh), 0x0004|0x0040) // NOZORDER|SHOWWINDOW
}

func workArea() (int, int, int, int) {
	var r rect
	ok, _, _ := pSysParams.Call(0x0030, 0, uintptr(unsafe.Pointer(&r)), 0)
	if ok == 0 {
		sw, _, _ := pGetMetrics.Call(0)
		sh, _, _ := pGetMetrics.Call(1)
		return 0, 0, int(sw), int(sh)
	}
	return int(r.l), int(r.t), int(r.r - r.l), int(r.b - r.t)
}

func arrangeWindows() int {
	p1, p2 := gameWindows()
	X, Y, W, H := workArea()
	n := 0
	switch cfg.Layout {
	case "stacked": // player 1 on top, player 2 below, each the full width
		for _, h := range p1 {
			place(h, X, Y, W, H/2, false)
			n++
		}
		for _, h := range p2 {
			place(h, X, Y+H/2, W, H-H/2, false)
			n++
		}
	default:
		keep := cfg.Layout != "side_fill"
		for _, h := range p1 {
			place(h, X, Y, W/2, H, keep)
			n++
		}
		for _, h := range p2 {
			place(h, X+W/2, Y, W/2, H, keep)
			n++
		}
	}
	return n
}

var arranged = map[uintptr]bool{}

// once both windows exist, snap them left/right a single time
func autoArrange() {
	p1, p2 := gameWindows()
	if len(p1) == 0 || len(p2) == 0 {
		return
	}
	for _, h := range append(p1, p2...) {
		if !arranged[h] {
			n := arrangeWindows()
			for _, x := range append(p1, p2...) {
				arranged[x] = true
			}
			logf("auto-arranged %d windows", n)
			return
		}
	}
}

func linkDir(src, dst string) (string, error) {
	out, err := exec.Command("cmd", "/c", "mklink", "/J", dst, src).CombinedOutput()
	return string(out), err
}

func openUI(url string) {
	for _, b := range []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	} {
		if exists(b) {
			if exec.Command(b, "--app="+url, "--window-size=1040,720").Start() == nil {
				return
			}
		}
	}
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func openPath(p string) {
	if p != "" {
		exec.Command("explorer", p).Start()
	}
}
