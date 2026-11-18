package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const Version = "v0.4.8"
const Title = "Cooperative Warfare launcher " + Version

//go:embed ui/*
var uiFS embed.FS

type Config struct {
	GameDir  string             `json:"gameDir"`
	P2Dir    string             `json:"p2Dir"`
	P2Name   string             `json:"p2Name"`
	P2ID     string             `json:"p2ID"`
	HostIP   string             `json:"hostIP"`
	P2Port   string             `json:"p2Port"`
	JoinIP   string             `json:"joinIP"`
	Arrange  bool               `json:"arrange"`
	Layout   string             `json:"layout"`
	Console  bool               `json:"console"`
	ExtraArg string             `json:"extraArgs"`
	Game     string             `json:"game"`
	S1Preset int                `json:"s1Preset"`
	Games    map[string]GameCfg `json:"games"`
	Bo1      Bo1Cfg             `json:"bo1"`
}

var (
	cfg      Config
	cfgPath  string
	exeDir   string
	logMu    sync.Mutex
	logLines []string
	logFile  string
	lastPing int64 // unix nanos, atomic
	byeAt    int64 // unix nanos of the last "page closed" beacon, atomic
)

func logf(format string, a ...interface{}) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, a...)
	logMu.Lock()
	logLines = append(logLines, line)
	if len(logLines) > 400 {
		logLines = logLines[len(logLines)-400:]
	}
	logMu.Unlock()
	if logFile != "" {
		if f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02"), line)
			f.Close()
		}
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func loadConfig() {
	cfg = Config{P2Name: "Loyalist", P2ID: "D152C402", P2Port: "27021", Arrange: true, Layout: "side_keep", Console: true, S1Preset: 5}
	if b, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(b, &cfg)
	}
	if cfg.Layout == "" {
		cfg.Layout = "side_keep"
	}
	if cfg.GameDir == "" {
		for _, c := range []string{exeDir, filepath.Dir(exeDir), "."} {
			if exists(filepath.Join(c, "iw5sp.exe")) {
				cfg.GameDir, _ = filepath.Abs(c)
				break
			}
		}
	}
	if cfg.P2Dir == "" && cfg.GameDir != "" {
		cfg.P2Dir = filepath.Join(filepath.Dir(cfg.GameDir), filepath.Base(cfg.GameDir)+"_P2")
	}
	if cfg.HostIP == "" {
		cfg.HostIP = bestIP()
	}
	initGames()
}

func saveConfig() {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(cfgPath, b, 0644)
}

func localIPs() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := i.Addrs()
		for _, a := range as {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, ipn.IP.String())
			}
		}
	}
	sort.Strings(out)
	return out
}

func bestIP() string {
	ips := localIPs()
	for _, ip := range ips {
		if strings.HasPrefix(ip, "26.") { // Radmin VPN range
			return ip
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return "127.0.0.1"
}

func state() map[string]interface{} {
	logMu.Lock()
	lines := append([]string{}, logLines...)
	logMu.Unlock()
	s := map[string]interface{}{
		"version": Version, "title": Title, "config": cfg, "ips": localIPs(), "log": lines, "os": runtime.GOOS,
	}
	g := cfg.GameDir
	s["hasGame"] = g != "" && exists(filepath.Join(g, "iw5sp.exe"))
	s["hasTekno"] = g != "" && exists(filepath.Join(g, "TeknoMW3_Client_Launcher.exe"))
	s["hasTeknoDll"] = g != "" && exists(filepath.Join(g, "TeknoMW3.dll"))
	s["p2Ready"] = cfg.P2Dir != "" && exists(filepath.Join(cfg.P2Dir, "iw5sp.exe"))
	n := 0
	if g != "" {
		for _, z := range []string{"innocent", "london", "hijack", "intro", "ny_manhattan", "berlin", "castle", "dubai", "hamburg", "ny_harbor", "paris_a", "paris_b", "payback", "prague", "prague_escape", "rescue_2", "warlord", "paris_ac130", "patch", "common"} {
			if exists(filepath.Join(g, "zone", "english", z+".ff.orig")) {
				n++
			}
		}
	}
	s["patchedZones"] = n
	s["games"] = gameStates()
	s["s1Presets"] = s1Presets
	w1, w2 := findWindows()
	s["win1"], s["win2"] = w1, w2
	return s
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(t, 0755)
		}
		return copyFile(p, t)
	})
}

// prepare builds the player 2 folder: same game files, big folders linked, own TeknoMW3 settings, exe import patched.
func prepare() error {
	g, p2 := cfg.GameDir, cfg.P2Dir
	if !exists(filepath.Join(g, "iw5sp.exe")) {
		return fmt.Errorf("iw5sp.exe not found in %s", g)
	}
	logf("preparing player 2 folder: %s", p2)
	if err := os.MkdirAll(p2, 0755); err != nil {
		return err
	}
	ents, _ := os.ReadDir(g)
	nfiles := 0
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(n), ".log") || n == "local_coop_log.txt" {
			continue
		}
		if err := copyFile(filepath.Join(g, n), filepath.Join(p2, n)); err != nil {
			logf("  skip %s (%v)", n, err)
			continue
		}
		nfiles++
	}
	logf("  copied %d files", nfiles)
	for _, d := range []string{"main", "zone", "miles", "raw", "userraw", "scripts", "devraw", "data", "mono-2.10.9", "mono-5.0.1.1", "Redist", "BanDB", "admin", "patches"} {
		src, dst := filepath.Join(g, d), filepath.Join(p2, d)
		if exists(src) && !exists(dst) {
			if out, err := linkDir(src, dst); err != nil {
				logf("  link %s failed: %v %s", d, err, out)
			}
		}
	}
	os.MkdirAll(filepath.Join(p2, "players2"), 0755)
	os.RemoveAll(filepath.Join(p2, "TeknoMW3"))
	if exists(filepath.Join(g, "TeknoMW3")) {
		if err := copyTree(filepath.Join(g, "TeknoMW3"), filepath.Join(p2, "TeknoMW3")); err != nil {
			logf("  TeknoMW3 folder copy: %v", err)
		}
	}
	if err := patchExe(filepath.Join(p2, "iw5sp.exe")); err != nil {
		logf("  exe patch: %v", err)
	}
	ini := fmt.Sprintf("[Settings]\r\nName=%s\r\nID=%s\r\nFOV=90\r\ncl_enableRCon=1\r\nmonkeytoy=1\r\n[Network]\r\nNetworkInterface=1\r\n[Info]\r\nName=%s\r\nID=%s\r\nClantag=\r\nTitle=\r\nClanTile=0\r\nClanEmblem=0\r\n", cfg.P2Name, cfg.P2ID, cfg.P2Name, cfg.P2ID)
	os.WriteFile(filepath.Join(p2, "teknogods.ini"), []byte(ini), 0644)
	os.MkdirAll(filepath.Join(p2, "TeknoMW3"), 0755)
	os.WriteFile(filepath.Join(p2, "TeknoMW3", "teknogods.ini"), []byte(ini), 0644)
	logf("player 2 folder ready")
	return nil
}

func patchExe(p string) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	find := []byte("steam_api.dll")
	rep := append([]byte("TeknoMW3.dll"), 0)
	i := indexOf(b, find)
	if i < 0 {
		if indexOf(b, []byte("TeknoMW3.dll")) >= 0 {
			logf("  exe already patched")
			return nil
		}
		return fmt.Errorf("steam_api.dll string not found")
	}
	copy(b[i:], rep)
	logf("  exe patched at 0x%X", i)
	return os.WriteFile(p, b, 0755)
}

func indexOf(b, sub []byte) int {
	for i := 0; i+len(sub) <= len(b); i++ {
		if b[i] == sub[0] && string(b[i:i+len(sub)]) == string(sub) {
			return i
		}
	}
	return -1
}

func startP1() error {
	l := filepath.Join(cfg.GameDir, "TeknoMW3_Client_Launcher.exe")
	if !exists(l) {
		return fmt.Errorf("TeknoMW3_Client_Launcher.exe not found in the game folder")
	}
	logf("starting player 1: %s", l)
	return startDetached(l, nil, cfg.GameDir)
}

func p2Args(ip string, port string) []string {
	a := []string{}
	if cfg.Console {
		a = append(a, "-console")
	}
	if port != "" {
		a = append(a, "+set", "net_port", port)
	}
	a = append(a, "+server", ip)
	if cfg.ExtraArg != "" {
		a = append(a, strings.Fields(cfg.ExtraArg)...)
	}
	return a
}

func startP2(ip, port string) error {
	if !exists(filepath.Join(cfg.P2Dir, "iw5sp.exe")) {
		return fmt.Errorf("player 2 folder is not prepared yet")
	}
	a := p2Args(ip, port)
	logf("starting game: iw5sp.exe %s", strings.Join(a, " "))
	return startDetached(filepath.Join(cfg.P2Dir, "iw5sp.exe"), a, cfg.P2Dir)
}

func startDetached(exe string, args []string, dir string) error {
	c := exec.Command(exe, args...)
	c.Dir = dir
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}

func restoreOriginals() int {
	zd := filepath.Join(cfg.GameDir, "zone", "english")
	n := 0
	ents, _ := os.ReadDir(zd)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".ff.orig") {
			dst := filepath.Join(zd, strings.TrimSuffix(e.Name(), ".orig"))
			if err := copyFile(filepath.Join(zd, e.Name()), dst); err == nil {
				n++
				logf("restored %s", filepath.Base(dst))
			}
		}
	}
	return n
}

func api(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ok := func(msg string) { json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "msg": msg}) }
	fail := func(err error) {
		logf("ERROR: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "msg": err.Error()})
	}
	switch strings.TrimPrefix(r.URL.Path, "/api/") {
	case "ping":
		atomic.StoreInt64(&lastPing, time.Now().UnixNano())
		ok("pong")
	case "bye":
		// the page was closed (or reloaded: a ping within a few seconds cancels the quit)
		atomic.StoreInt64(&byeAt, time.Now().UnixNano())
		ok("bye")
	case "state":
		json.NewEncoder(w).Encode(state())
	case "config":
		var c Config
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			fail(err)
			return
		}
		cfg = c
		initGames()
		if cfg.P2Dir == "" && cfg.GameDir != "" {
			cfg.P2Dir = filepath.Join(filepath.Dir(cfg.GameDir), filepath.Base(cfg.GameDir)+"_P2")
		}
		saveConfig()
		arrangeWindows()
		ok("saved")
	case "prepare":
		if err := prepare(); err != nil {
			fail(err)
			return
		}
		ok("player 2 folder ready")
	case "start1":
		if err := startP1(); err != nil {
			fail(err)
			return
		}
		ok("player 1 started")
	case "start2":
		if err := startP2(cfg.HostIP, cfg.P2Port); err != nil {
			fail(err)
			return
		}
		ok("player 2 started")
	case "join":
		var q struct{ IP string }
		json.NewDecoder(r.Body).Decode(&q)
		ip := strings.TrimSpace(q.IP)
		if ip == "" {
			fail(fmt.Errorf("enter the host's address first"))
			return
		}
		cfg.JoinIP = ip
		saveConfig()
		// connecting to someone else: no extra port override, same +server command
		if err := startP2(ip, ""); err != nil {
			fail(err)
			return
		}
		ok("connecting to " + ip)
	case "launch":
		var q struct{ Game string }
		json.NewDecoder(r.Body).Decode(&q)
		if err := launchGame(q.Game); err != nil {
			fail(err)
			return
		}
		ok(gameNames[q.Game] + " started")
	case "bo1":
		var q struct {
			Action string
			Player int
		}
		json.NewDecoder(r.Body).Decode(&q)
		var err error
		switch q.Action {
		case "host":
			err = bo1Start(1, true)
		case "lobby":
			err = bo1Start(1, false)
		case "join":
			err = bo1Start(q.Player, false)
		case "all":
			err = bo1Everything()
		default:
			err = fmt.Errorf("unknown action %q", q.Action)
		}
		if err != nil {
			fail(err)
			return
		}
		ok("started")
	case "arrange":
		n := arrangeWindows()
		logf("arranged %d windows (left = player 1, right = player 2)", n)
		ok(fmt.Sprintf("%d windows arranged", n))
	case "restore":
		n := restoreOriginals()
		ok(fmt.Sprintf("restored %d zone files", n))
	case "s1patch":
		var q struct{ Set string }
		json.NewDecoder(r.Body).Decode(&q)
		g := cfg.Games["s1"]
		if err := s1Install(g.Dir, q.Set); err != nil {
			fail(err)
			return
		}
		ok("S1 patch " + q.Set + " installed")
	case "s1restore":
		n := s1Restore(cfg.Games["s1"].Dir)
		ok(fmt.Sprintf("S1 originals restored (%d files)", n))
	case "open":
		var q struct{ What string }
		json.NewDecoder(r.Body).Decode(&q)
		switch q.What {
		case "game":
			openPath(cfg.GameDir)
		case "log":
			openPath(logFile)
		case "s1log":
			openPath(filepath.Join(exeDir, "s1_run_log.txt"))
		case "bo1":
			openPath(cfg.Bo1.Game)
		case "bo1plut":
			openPath(cfg.Bo1.Plut)
		case "nx1", "s1":
			openPath(cfg.Games[q.What].Dir)
		}
		ok("opened")
	default:
		http.NotFound(w, r)
	}
}

func main() {
	exe, _ := os.Executable()
	exeDir = filepath.Dir(exe)
	cfgPath = filepath.Join(exeDir, "coop_launcher.json")
	logFile = filepath.Join(exeDir, "coop_launcher_log.txt")
	loadConfig()
	logf("%s started", Title)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println(err)
		return
	}
	sub, _ := fs.Sub(uiFS, "ui")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/", api)
	url := "http://" + ln.Addr().String() + "/"
	fmt.Println("==== " + Title + " ====")
	fmt.Println("UI at", url, "(close the window to quit)")
	go func() {
		time.Sleep(300 * time.Millisecond)
		openUI(url)
	}()
	go func() {
		for {
			time.Sleep(3 * time.Second)
			lp, bye := atomic.LoadInt64(&lastPing), atomic.LoadInt64(&byeAt)
			now := time.Now().UnixNano()
			// closed: a bye beacon arrived, nothing has pinged since, and 8 s have passed
			if bye != 0 && lp < bye && now-bye > int64(8*time.Second) {
				os.Exit(0)
			}
			// safety net only: a frozen / throttled background window stops pinging, so never quit on a short silence
			if lp != 0 && now-lp > int64(6*time.Hour) {
				os.Exit(0)
			}
			if cfg.Arrange {
				autoArrange()
			}
		}
	}()
	http.Serve(ln, mux)
}
