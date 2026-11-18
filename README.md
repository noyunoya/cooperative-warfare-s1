[README.md](https://github.com/user-attachments/files/35045960/README.md)
# Cooperative Warfare - S1 (Blacksmith SP) co-op tools

Co-op (2-player split screen) work for the S1 single-player build running under ReXGlue: Fusion launched from the Spec Ops menu, script hooks for Player 2, launcher.

**No game data is in this repo**: no `.ff`, `.bik`, `.xex`, `.map`/`.pdb`, no recompiled executables, and no game scripts. You need your own copy of the game.

- `launcher/` - CoopLauncher source (Go). Patch sets are not embedded; build them with `tools/` and place them in `launcher/s1patch/<set>/`.
- `tools/` - fastfile builder (`mkff.py`, `build18.py`), spec ops table codec (`mktable.py`), script extractor (`extract_scripts.py`).
- `hooks/` - our Player 2 script hook (decoded builtin names).
- `notes/` - project notes and the S1 builtin ID map.

Third-party: see `THIRD_PARTY.md`. Licence for our own code: MIT (see LICENSE). Game data and third-party software are not covered; see THIRD_PARTY.md.
