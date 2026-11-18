package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GameCfg is one extra project hosted in the launcher (Black Ops, Nexus, Blacksmith).
type GameCfg struct {
	Exe  string `json:"exe"`
	Dir  string `json:"dir"`
	Args string `json:"args"`
}

var gameNames = map[string]string{"bo1": "Black Ops", "nx1": "Nexus", "s1": "Blacksmith"}

func firstExisting(c ...string) string {
	for _, p := range c {
		if p != "" && exists(p) {
			return p
		}
	}
	return ""
}

func initGames() {
	initBo1()
	if cfg.Games == nil {
		cfg.Games = map[string]GameCfg{}
	}
	home, _ := os.UserHomeDir()
	desk := filepath.Join(home, "Desktop")
	def := map[string]GameCfg{
		"s1": {
			Exe: firstExisting(filepath.Join(exeDir, "s1_sp.exe"),
				filepath.Join(desk, "S1 exe", "s1_sp", "out", "build", "win-amd64", "RelWithDebInfo", "s1_sp.exe")),
			Dir: filepath.Join(desk, "Canary", "Games", "s1"),
		},
		"nx1": {
			Exe: firstExisting(filepath.Join(desk, "Canary", "Games", "NX1", "nx1_sp.exe"),
				filepath.Join(desk, "Canary", "nx1_sp.exe")),
			Dir: filepath.Join(desk, "Canary", "Games", "NX1"),
		},
		"bo1": {
			Exe: firstExisting(`C:\Program Files (x86)\Steam\steamapps\common\Call of Duty Black Ops\BlackOps.exe`),
			Dir: `C:\Program Files (x86)\Steam\steamapps\common\Call of Duty Black Ops`,
		},
	}
	for k, d := range def {
		g := cfg.Games[k]
		if g.Exe == "" {
			g.Exe = d.Exe
		}
		if g.Dir == "" {
			g.Dir = d.Dir
		}
		cfg.Games[k] = g
	}
	if cfg.Game == "" {
		cfg.Game = "mw3"
	}
}

func gameStates() map[string]interface{} {
	out := map[string]interface{}{}
	for k, g := range cfg.Games {
		out[k] = map[string]interface{}{"hasExe": g.Exe != "" && exists(g.Exe), "hasDir": g.Dir != "" && exists(g.Dir)}
	}
	out["bo1x"] = bo1State()
	if g, ok := cfg.Games["s1"]; ok && g.Dir != "" {
		out["s1"].(map[string]interface{})["hasPatched"] = exists(filepath.Join(g.Dir, "default_patched.xex"))
		out["s1"].(map[string]interface{})["patchSet"] = s1Installed(g.Dir)
		out["s1"].(map[string]interface{})["hasLoadVideo"] = exists(filepath.Join(g.Dir, "fusion_load.bik"))
	}
	return out
}

// GPU presets for the S1 recompile (written into s1_sp.toml; the exe rejects these as command-line options).
var s1Presets = []string{
	"default GPU settings",
	"ROV render targets, async compilation ON (try first)",
	"ROV + async compilation OFF (slow first run, leave 5+ min)",
	"ROV + async OFF + NX1 settings + submit_on_primary_buffer_end",
	"ROV, async ON + readback_memexport (tested: no change)",
	"ROV, async ON + readback_resolve 'fast' (RECOMMENDED: fixes the white-out)",
	"ROV, async ON + use_fuzzy_alpha_epsilon",
	"ROV, async ON + native_2x_msaa OFF",
	"ROV, async ON + float24 depth convert + round",
}

// Keyboard + mouse layout (Call of Duty PC style) mapped onto the pad buttons the game expects.
const s1Keybinds = "keybind_right_trigger = 'MouseLeft'\nkeybind_left_trigger = 'MouseRight'\nkeybind_lstick_press = 'LeftShift'\nkeybind_b = 'LeftCtrl'\nkeybind_x = 'R,E'\nkeybind_rstick_press = 'V'\nkeybind_right_shoulder = 'G'\nkeybind_left_shoulder = 'Q'\nkeybind_a = 'Space'\nkeybind_y = '1'\nkeybind_start = 'Escape'\nkeybind_back = 'Tab'\n"

func s1WriteToml(g GameCfg) (string, error) {
	exeDirS1 := filepath.Dir(g.Exe)
	fw := func(p string) string { return strings.ReplaceAll(p, `\`, "/") }
	var b strings.Builder
	fmt.Fprintf(&b, "[app]\nroot = '%s'\n[hid]\nmnk_mode = true\nmnk_mouse = true\nmnk_sensitivity = 6.0\n%s[log]\nlevel = 'debug'\n[runtime]\nprotect_zero = false\n[title]\ngame_root = '%s'\nupdate_root = '%s'\n", fw(exeDirS1), s1Keybinds, fw(g.Dir), fw(g.Dir))
	p := cfg.S1Preset
	if p > 0 && p < len(s1Presets) {
		for _, sec := range []string{"gpu.xenos", "gpu.d3d12"} {
			fmt.Fprintf(&b, "[%s]\n", sec)
			if sec == "gpu.d3d12" {
				b.WriteString("render_target_path = 'rov'\n")
			}
			if (p == 2 || p == 3) && sec == "gpu.xenos" {
				b.WriteString("async_shader_compilation = false\n")
			}
			switch p {
			case 4:
				b.WriteString("readback_memexport = true\n")
			case 5:
				if sec == "gpu.xenos" {
					b.WriteString("readback_resolve = 'fast'\n")
				}
			case 6:
				b.WriteString("use_fuzzy_alpha_epsilon = true\n")
			case 7:
				b.WriteString("native_2x_msaa = false\n")
			case 8:
				b.WriteString("depth_float24_convert_in_pixel_shader = true\ndepth_float24_round = true\n")
			}
			if p == 3 {
				if sec == "gpu.d3d12" {
					b.WriteString("submit_on_primary_buffer_end = true\n")
				} else {
					for _, k := range []string{"direct_host_resolve", "gamma_render_target_as_unorm16", "half_pixel_offset", "mrt_edram_used_range_clamp_to_min", "pre_mask_resolve_l2_block", "resolve_resolution_scale_fill_half_pixel_offset", "snorm16_render_target_full_range"} {
						fmt.Fprintf(&b, "%s = false\n", k)
					}
				}
			}
		}
	}
	return filepath.Join(exeDirS1, "s1_sp.toml"), os.WriteFile(filepath.Join(exeDirS1, "s1_sp.toml"), []byte(b.String()), 0644)
}

func launchGame(k string) error {
	g, ok := cfg.Games[k]
	if !ok {
		return fmt.Errorf("unknown game %q", k)
	}
	if g.Exe == "" || !exists(g.Exe) {
		return fmt.Errorf("%s: exe not found - set its path in the form and press Save", gameNames[k])
	}
	args := []string{}
	if k == "s1" {
		if _, err := os.Stat(filepath.Join(g.Dir, "default_patched.xex")); err != nil {
			logf("warning: default_patched.xex not found in %s", g.Dir)
		}
		t, err := s1WriteToml(g)
		if err != nil {
			return err
		}
		logf("S1 preset %d (%s) written to %s", cfg.S1Preset, s1Presets[cfg.S1Preset], t)
		args = []string{"--game_data_root", g.Dir, "--update_data_root", g.Dir, "--protect_zero", "false", "--mnk_mode", "true", "--log_level", "debug"}
	}
	if g.Args != "" {
		args = append(args, strings.Fields(g.Args)...)
	}
	c := exec.Command(g.Exe, args...)
	c.Dir = filepath.Dir(g.Exe)
	if k == "s1" {
		lf := filepath.Join(exeDir, "s1_run_log.txt")
		if f, err := os.Create(lf); err == nil {
			c.Stdout, c.Stderr = f, f
			logf("S1 console output -> %s", lf)
		}
	}
	logf("starting %s: %s %s", gameNames[k], g.Exe, strings.Join(args, " "))
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}

// ---- Black Ops 1 (Plutonium t5sp) co-op: same commands as bo1_coop_launch.bat ----

type Bo1Cfg struct {
	Game    string    `json:"game"`
	Plut    string    `json:"plut"`
	Mod     string    `json:"mod"`
	Map     string    `json:"map"`
	Port    string    `json:"port"`
	Names   [4]string `json:"names"`
	Players int       `json:"players"`
	Connect string    `json:"connect"`
}

func initBo1() {
	b := &cfg.Bo1
	if b.Game == "" {
		b.Game = `C:\Program Files (x86)\Steam\steamapps\common\Call of Duty Black Ops`
	}
	if b.Plut == "" {
		la := os.Getenv("LOCALAPPDATA")
		if la == "" {
			home, _ := os.UserHomeDir()
			la = filepath.Join(home, "AppData", "Local")
		}
		b.Plut = filepath.Join(la, "Plutonium")
	}
	if b.Mod == "" {
		b.Mod = "mods/CoOp"
	}
	if b.Map == "" {
		b.Map = "cuba"
	}
	if b.Port == "" {
		b.Port = "28960"
	}
	def := [4]string{"Mason", "Woods", "Bowman", "Carlos"}
	for i := range b.Names {
		if b.Names[i] == "" {
			b.Names[i] = def[i]
		}
	}
	if b.Players < 2 || b.Players > 4 {
		b.Players = 2
	}
	if b.Connect == "" {
		b.Connect = "127.0.0.1"
	}
}

func bo1Boot() string { return filepath.Join(cfg.Bo1.Plut, "bin", "plutonium-bootstrapper-win32.exe") }

func bo1State() map[string]interface{} {
	b := cfg.Bo1
	modName := filepath.Base(strings.ReplaceAll(b.Mod, "/", string(filepath.Separator)))
	return map[string]interface{}{
		"hasBoot": exists(bo1Boot()),
		"hasGame": exists(filepath.Join(b.Game, "BlackOps.exe")),
		"hasMod":  exists(filepath.Join(b.Plut, "storage", "t5", "mods", modName)),
	}
}

// bo1Start launches player i (1 = host). withMap: host straight into the map; false = main menu only (open the lobby).
func bo1Start(i int, withMap bool) error {
	b := cfg.Bo1
	if !exists(bo1Boot()) {
		return fmt.Errorf("Plutonium bootstrapper not found: %s", bo1Boot())
	}
	if !exists(filepath.Join(b.Game, "BlackOps.exe")) {
		return fmt.Errorf("BlackOps.exe not found in %s", b.Game)
	}
	if i < 1 || i > 4 {
		return fmt.Errorf("player %d is out of range", i)
	}
	port, _ := strconv.Atoi(b.Port)
	if port == 0 {
		port = 28960
	}
	n := strconv.Itoa(b.Players)
	name := b.Names[i-1]
	args := []string{"t5sp", b.Game, "-lan", "+name", name, "+set", "fs_game", b.Mod}
	if i == 1 {
		args = append(args, "+set", "net_port", strconv.Itoa(port), "+set", "sp_minplayers", n, "+set", "party_minplayers", n)
		if withMap {
			args = append(args, "+map", b.Map)
		}
	} else {
		args = append(args, "+set", "net_port", strconv.Itoa(port+i-1), "+connect", b.Connect+":"+strconv.Itoa(port))
	}
	logf("BO1 player %d (%s): %s", i, name, strings.Join(args, " "))
	c := exec.Command(bo1Boot(), args...)
	c.Dir = b.Plut
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}

func bo1Everything() error {
	if err := bo1Start(1, true); err != nil {
		return err
	}
	n := cfg.Bo1.Players
	go func() {
		logf("BO1: waiting 25 s for the host to start...")
		time.Sleep(25 * time.Second)
		for i := 2; i <= n; i++ {
			if err := bo1Start(i, false); err != nil {
				logf("ERROR: %v", err)
				return
			}
			time.Sleep(15 * time.Second)
		}
	}()
	return nil
}
