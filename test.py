"""Tests for dot: one test per protected behavior. Runs offline in temp folders (HOME, DOT_HOME and a
local bare git repo for sync); run with `python3 test.py`."""
import os, plistlib, shutil, stat, subprocess, sys, tempfile, unittest
from pathlib import Path

DOT = str(Path(__file__).resolve().parent / "dot")


class Dot(unittest.TestCase):
    """Each test gets a fresh temp home with the setup at ~/.dot and machine name `laptop`."""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.tmp)
        self.home = self.tmp / "home"
        self.setup_dir = self.home / ".dot"
        self.setup_dir.mkdir(parents=True)
        self.env = {**os.environ, "HOME": str(self.home), "DOT_HOME": str(self.setup_dir), "DOT_MACHINE": "laptop",
                    "GIT_CONFIG_NOSYSTEM": "1", "GIT_AUTHOR_NAME": "test", "GIT_AUTHOR_EMAIL": "test@example.com",
                    "GIT_COMMITTER_NAME": "test", "GIT_COMMITTER_EMAIL": "test@example.com",
                    "GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "init.defaultBranch", "GIT_CONFIG_VALUE_0": "main"}

    def write(self, root, files):
        """Write {relative path: text} under `root`."""
        for rel, text in files.items():
            p = Path(root, rel)
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(text)

    def config(self, toml, **files):
        """Write dot.toml and source files (keyword names use __ for /) into the setup."""
        self.write(self.setup_dir, {"dot.toml": toml, **{k.replace("__", "/"): v for k, v in files.items()}})

    def dot(self, *args, machine="laptop"):
        """Run dot; return (exit code, stdout + stderr)."""
        r = subprocess.run([sys.executable, DOT, *args], env={**self.env, "DOT_MACHINE": machine},
                           capture_output=True, text=True)
        return r.returncode, r.stdout + r.stderr

    def live(self, rel):
        """Text of a file in the temp home, or None if it does not exist."""
        p = self.home / rel
        return p.read_text() if p.exists() else None

    def git(self, cwd, *args):
        """Run git in `cwd`, failing the test on error."""
        return subprocess.run(["git", *args], cwd=cwd, env=self.env, check=True, capture_output=True, text=True).stdout

    def remote(self, toml, **files):
        """Create a bare remote holding this setup and clone it to ~/.dot; return the bare repo path."""
        bare, seed = self.tmp / "remote.git", self.tmp / "seed"
        self.git(self.tmp, "init", "--bare", "-q", str(bare))
        self.git(self.tmp, "clone", "-q", str(bare), str(seed))
        self.write(seed, {"dot.toml": toml, ".gitignore": ".state/\n", **files})
        self.git(seed, "add", "-A")
        self.git(seed, "commit", "-qm", "setup")
        self.git(seed, "push", "-q", "-u", "origin", "main")
        shutil.rmtree(self.setup_dir)
        self.git(self.tmp, "clone", "-q", str(bare), str(self.setup_dir))
        return bare

    def commit(self, cwd, files, message="change"):
        """Write files into a clone and commit them."""
        self.write(cwd, files)
        self.git(cwd, "add", "-A")
        self.git(cwd, "commit", "-qm", message)

    # Rendering and mapping

    def test_template_renders_machine_overrides(self):
        self.config('[values]\nroot = "~/Code"\n[machine.server]\nroot = "~/src"\n[templates]\n"t.md" = "~/t.md"\n',
                    **{"t.md": "{{machine}} {{root}}\n"})
        self.dot("apply")
        self.assertEqual(self.live("t.md"), "laptop ~/Code\n")
        self.dot("apply", machine="server")
        self.assertEqual(self.live("t.md"), "server ~/src\n")

    def test_source_fans_out_to_every_destination(self):
        self.config('[files]\n"a.txt" = ["~/x/a.txt", "~/y/a.txt"]\n', **{"a.txt": "a\n"})
        self.assertEqual(self.dot("apply")[0], 0)
        self.assertEqual((self.live("x/a.txt"), self.live("y/a.txt")), ("a\n", "a\n"))

    def test_status_shows_exactly_what_apply_does(self):
        self.config('[files]\n"a" = "~/a"\n"b" = "~/b"\n', a="1", b="1")
        self.dot("apply")
        self.config('[files]\n"a" = "~/a"\n"c" = "~/c"\n', a="2", c="1")
        code, out = self.dot("status")
        self.assertEqual(code, 1)
        self.assertEqual(out.splitlines(), ["- removed     ~/b", "~ changed     ~/a", "+ new         ~/c"])
        self.assertEqual(self.dot("apply")[1], out)
        self.assertEqual(self.dot("status"), (0, "up to date\n"))

    def test_write_keeps_existing_mode_and_new_file_takes_source_mode(self):
        self.config('[files]\n"run" = "~/bin/run"\n', run="1")
        os.chmod(self.setup_dir / "run", 0o755)
        self.dot("apply")
        self.assertEqual(stat.S_IMODE(os.stat(self.home / "bin/run").st_mode), 0o755)
        os.chmod(self.home / "bin/run", 0o700)
        (self.setup_dir / "run").write_text("2")
        self.dot("apply")
        self.assertEqual(self.live("bin/run"), "2")
        self.assertEqual(stat.S_IMODE(os.stat(self.home / "bin/run").st_mode), 0o700)

    def test_symlink_in_source_is_copied_as_symlink(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__real": "x"})
        os.symlink("real", self.setup_dir / "d/link")
        self.dot("apply")
        self.assertEqual(os.readlink(self.home / "d/link"), "real")

    def test_machines_limits_a_mapping(self):
        self.config('[files]\n"a" = { to = "~/a", machines = ["server"] }\n', a="1")
        self.dot("apply")
        self.assertIsNone(self.live("a"))
        self.dot("apply", machine="server")
        self.assertEqual(self.live("a"), "1")

    def test_machine_name_in_source_path(self):
        self.config('[machine.server]\n[files]\n"cfg.{{machine}}.json" = "~/cfg.json"\n',
                    **{"cfg.laptop.json": "laptop", "cfg.server.json": "server"})
        self.dot("apply")
        self.assertEqual(self.live("cfg.json"), "laptop")

    def test_new_folder_takes_source_mode(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__private__f": "1"})
        os.chmod(self.setup_dir / "d/private", 0o700)
        self.dot("apply")
        self.assertEqual(stat.S_IMODE(os.stat(self.home / "d/private").st_mode), 0o700)

    def test_file_becoming_folder_converges(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__x": "1"})
        self.dot("apply")
        (self.setup_dir / "d/x").unlink()
        self.write(self.setup_dir, {"d/x/f": "2"})
        self.assertEqual(self.dot("apply")[0], 0)
        self.assertEqual(self.live("d/x/f"), "2")
        self.assertEqual(self.dot("status"), (0, "up to date\n"))

    def test_folder_becoming_file_converges(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__x__f": "1"})
        (self.setup_dir / "d/empty").mkdir()
        self.dot("apply")
        shutil.rmtree(self.setup_dir / "d/x")
        (self.setup_dir / "d/empty").rmdir()
        self.write(self.setup_dir, {"d/x": "2", "d/empty": "3"})
        self.assertEqual(self.dot("apply")[0], 0)
        self.assertEqual((self.live("d/x"), self.live("d/empty")), ("2", "3"))
        self.assertEqual(self.dot("status"), (0, "up to date\n"))

    def test_status_reports_differing_binary_files(self):
        self.config('[files]\n"b" = "~/b"\n', b="")
        (self.setup_dir / "b").write_bytes(b"\xff\x00")
        (self.home / "b").write_bytes(b"\xfe")
        self.assertIn("binary files live ~/b and dot ~/b differ", self.dot("status", "~/b")[1])

    # Files edited in place

    def test_folder_replaced_by_symlink_is_not_written_through(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__f": "1", "d__g": "1"})
        self.dot("apply")
        shutil.move(self.home / "d", self.tmp / "outside")
        os.symlink(self.tmp / "outside", self.home / "d")
        (self.setup_dir / "d/f").write_text("2")
        (self.setup_dir / "d/g").unlink()
        self.assertEqual(self.dot("apply")[0], 1)
        self.assertEqual(sorted(os.listdir(self.tmp / "outside")), ["f", "g"])
        self.assertEqual((self.tmp / "outside/f").read_text(), "1")

    def test_edited_file_skipped_by_sync_and_refused_by_apply_until_force(self):
        self.config('[files]\n"a" = "~/a"\n', a="1")
        self.git(self.setup_dir, "init", "-q")
        self.commit(self.setup_dir, {".gitignore": ".state/\n"})
        self.dot("apply")
        (self.home / "a").write_text("mine")
        self.commit(self.setup_dir, {"a": "2"})
        self.dot("sync")
        self.assertEqual(self.live("a"), "mine")
        self.assertIn("edited here: ~/a (dot take ~/a, or dot apply --force)", (self.setup_dir / ".state/sync.log").read_text())
        self.assertEqual(self.dot("apply")[0], 1)
        self.assertEqual(self.live("a"), "mine")
        self.assertEqual(self.dot("apply", "--force")[0], 0)
        self.assertEqual(self.live("a"), "2")

    def test_take_copies_live_edit_back_to_source(self):
        self.config('[files]\n"d" = ["~/d", "~/e"]\n', **{"d__f": "1"})
        self.dot("apply")
        (self.home / "d/f").write_text("2")
        (self.home / "d/g").write_text("new")
        self.assertEqual(self.dot("take", "~/d")[0], 0)
        self.assertEqual(((self.setup_dir / "d/f").read_text(), (self.setup_dir / "d/g").read_text()), ("2", "new"))

    def test_take_refuses_template_destination(self):
        self.config('[templates]\n"t" = "~/t"\n', t="{{machine}}\n")
        self.dot("apply")
        (self.home / "t").write_text("edited\n")
        code, out = self.dot("take", "~/t")
        self.assertEqual(code, 1)
        self.assertIn("+edited", out)
        self.assertEqual((self.setup_dir / "t").read_text(), "{{machine}}\n")

    def test_take_of_missing_folder_is_an_error(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__f": "1"})
        self.dot("apply")
        shutil.rmtree(self.home / "d")
        self.assertEqual(self.dot("take", "~/d"), (2, "dot: ~/d does not exist\n"))

    # Deletion

    def test_mirror_deletes_extra_files_and_empty_folders(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__f": "1"})
        self.dot("apply")
        self.write(self.home, {"d/extra": "x", "d/sub/extra": "x", "d/keep.tmp": "x"})
        self.config('exclude = ["*.tmp"]\n[files]\n"d" = "~/d"\n')
        self.dot("apply")
        self.assertEqual(sorted(os.listdir(self.home / "d")), ["f", "keep.tmp"])

    def test_mirror_off_keeps_and_reports_extra_files(self):
        self.config('[files]\n"d" = { to = "~/d", mirror = false }\n', **{"d__f": "1"})
        self.dot("apply")
        self.write(self.home, {"d/extra": "x"})
        self.assertEqual(self.dot("status"), (0, "? extra       ~/d/extra\n"))
        self.dot("apply")
        self.assertEqual(self.live("d/extra"), "x")

    def test_dropped_file_is_deleted(self):
        self.config('[files]\n"a" = "~/a"\n"d" = "~/x/d"\n', a="1", **{"d__f": "1"})
        self.dot("apply")
        self.config('[files]\n"a" = "~/a"\n')
        self.dot("apply")
        self.assertEqual(sorted(os.listdir(self.home)), [".dot", "a", "x"])
        self.assertEqual(os.listdir(self.home / "x"), [])

    def test_dropped_file_edited_here_is_kept_and_reported(self):
        self.config('[files]\n"a" = "~/a"\n"b" = "~/b"\n', a="1", b="1")
        self.dot("apply")
        (self.home / "b").write_text("mine")
        self.config('[files]\n"a" = "~/a"\n')
        self.assertEqual(self.dot("apply")[0], 1)
        self.assertEqual(self.live("b"), "mine")
        self.assertIn("! edited here ~/b", self.dot("status")[1])

    def test_newly_excluded_file_is_kept(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__f": "1", "d__g": "1"})
        self.dot("apply")
        self.config('[files]\n"d" = { to = "~/d", exclude = ["g"] }\n')
        self.dot("apply")
        self.assertEqual(self.live("d/g"), "1")

    def test_prune_keeps_folder_still_in_source(self):
        self.config('[files]\n"d" = "~/d"\n', **{"d__sub__f": "1"})
        self.dot("apply")
        (self.setup_dir / "d/sub/f").unlink()
        self.dot("apply")
        self.assertTrue((self.home / "d/sub").is_dir())
        self.assertEqual(self.dot("status"), (0, "up to date\n"))

    # Validation

    def assertInvalid(self, toml, message, **files):
        self.config(toml, **files)
        code, out = self.dot("status")
        self.assertEqual(code, 2)
        self.assertIn(message, out)

    def test_undefined_value_is_an_error(self):
        self.assertInvalid('[templates]\n"t" = "~/t"\n', 'dot: ~/.dot/t on laptop: undefined name "nope"', t="{{nope}}")

    def test_missing_source_on_another_machine_is_an_error(self):
        self.assertInvalid('[machine.server]\n[files]\n"c.{{machine}}" = "~/c"\n',
                           'dot: dot.toml: [files] "c.{{machine}}" on server: missing source ~/.dot/c.server', **{"c.laptop": ""})

    def test_two_mappings_writing_one_destination_is_an_error(self):
        self.assertInvalid('[files]\n"a" = "~/a"\n"b" = "~/a"\n', '[files] "b" on laptop: ~/a is also written by [files] "a"', a="", b="")

    def test_nested_destinations_are_an_error(self):
        self.assertInvalid('[files]\n"a" = "~/d/a"\n"d" = "~/d"\n', '[files] "a" on laptop: ~/d/a is inside ~/d', a="", **{"d__f": ""})

    def test_collision_through_symlinked_parent_is_an_error(self):
        (self.home / "real").mkdir()
        os.symlink(self.home / "real", self.home / "alias")
        self.assertInvalid('[files]\n"a" = "~/alias/f"\n"b" = "~/real/f"\n', "~/real/f is also written by", a="", b="")

    def test_destination_inside_root_folder_is_an_error(self):
        self.assertInvalid('[files]\n"a" = "/"\n"b" = "/nested"\n', '[files] "b" on laptop: /nested is inside /', a="", b="")

    def test_wrong_config_types_are_errors(self):
        self.assertInvalid('[files]\n"a" = { to = "~/a", mirror = "false" }\n',
                           'dot: dot.toml: [files] "a": mirror must be true or false', a="")
        self.assertInvalid('version = "1"\n', "dot: dot.toml: version must be a whole number")

    def test_unknown_mapping_key_is_an_error(self):
        self.assertInvalid('[files]\n"a" = { to = "~/a", mirorr = false }\n', 'dot: dot.toml: [files] "a": unknown key "mirorr"', a="")

    # Sync

    def test_sync_fast_forwards_and_applies(self):
        bare = self.remote('[files]\n"a" = "~/a"\n', a="1")
        other = self.tmp / "other"
        self.git(self.tmp, "clone", "-q", str(bare), str(other))
        self.commit(other, {"a": "2"})
        self.git(other, "push", "-q")
        self.assertEqual(self.dot("sync"), (0, ""))
        self.assertEqual(self.live("a"), "2")

    def test_sync_pushes_when_ahead_and_push_is_on(self):
        bare = self.remote('[sync]\npush = true\n[files]\n"a" = "~/a"\n', a="1")
        self.commit(self.setup_dir, {"a": "2"}, "local")
        self.dot("sync")
        self.assertEqual(self.git(bare, "log", "-1", "--format=%s").strip(), "local")

    def test_sync_does_not_push_when_push_is_off(self):
        bare = self.remote('[sync]\npush = false\n[files]\n"a" = "~/a"\n', a="1")
        self.commit(self.setup_dir, {"a": "2"}, "local")
        self.dot("sync")
        self.assertEqual(self.git(bare, "log", "-1", "--format=%s").strip(), "setup")
        self.assertEqual(self.live("a"), "2")

    def test_sync_refuses_uncommitted_changes(self):
        self.remote('[files]\n"a" = "~/a"\n', a="1")
        (self.setup_dir / "a").write_text("dirty")
        self.assertEqual(self.dot("sync")[0], 1)
        self.assertIsNone(self.live("a"))
        self.assertIn("refused: uncommitted changes", (self.setup_dir / ".state/last").read_text())


    # Install

    def install(self):
        """Run dot install with stub systemctl and launchctl first on PATH; return the timer's
        environment as {name: value} on macOS, or the systemd service text on Linux."""
        stubs = self.tmp / "bin"
        stubs.mkdir()
        for name in ("systemctl", "launchctl"):
            (stubs / name).write_text("#!/bin/sh\nexit 0\n")
            os.chmod(stubs / name, 0o755)
        self.env["PATH"] = f"{stubs}{os.pathsep}{self.env['PATH']}"
        self.assertEqual(self.dot("install")[0], 0)
        if sys.platform == "darwin":
            return plistlib.loads((self.home / "Library/LaunchAgents/dot.plist").read_bytes())["EnvironmentVariables"]
        return (self.home / ".config/systemd/user/dot.service").read_text()

    def test_install_keeps_machine_name(self):
        self.config("")
        timer = self.install()
        if sys.platform == "darwin":
            self.assertEqual(timer["DOT_MACHINE"], "laptop")
        else:
            self.assertIn('Environment="DOT_MACHINE=laptop"\n', timer)

    @unittest.skipUnless(sys.platform.startswith("linux"), "systemd units are Linux only")
    def test_systemd_unit_escapes_values(self):
        self.setup_dir = self.home / 'a%b"c\\d'
        self.env["DOT_HOME"] = str(self.setup_dir)
        self.config("")
        self.assertIn(f'Environment="DOT_HOME={self.home}/a%%b\\"c\\\\d"\n', self.install())


if __name__ == "__main__":
    unittest.main()
