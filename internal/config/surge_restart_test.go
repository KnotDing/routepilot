package config

import "testing"

func TestSurgeRestartMigrationValidationAndClone(t *testing.T) {
	d := Default()
	if d.Settings.Surge.Restart.Enabled || d.Settings.Surge.Restart.Effective().Failures != 5 || d.Validate() != nil {
		t.Fatal("unsafe migration")
	}
	s := d.Settings.Surge.Restart.Effective()
	s.Enabled = true
	s.API = true
	d.Settings.Surge.Restart = s
	if d.Validate() == nil {
		t.Fatal("API URL missing")
	}
	d.Settings.Surge.APIURL = "http://127.0.0.1:6171"
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*SurgeRestart){func(s *SurgeRestart) { s.Failures = 1 }, func(s *SurgeRestart) { s.CooldownSeconds = 1 }, func(s *SurgeRestart) { s.MaxPerHour = 11 }, func(s *SurgeRestart) { s.Groups = []string{"a", "a"} }, func(s *SurgeRestart) { s.Nodes = []string{"a\nb"} }, func(s *SurgeRestart) { s.ProbeURL = "https://user:pass@example.test" }} {
		bad := d.Clone()
		edit(&bad.Settings.Surge.Restart)
		if bad.Validate() == nil {
			t.Fatal("bad tolerance or target accepted")
		}
	}
	d.Settings.Surge.Restart.Groups = []string{"Group"}
	d.Settings.Surge.Restart.Nodes = []string{"Node"}
	c := d.Clone()
	c.Settings.Surge.Restart.Groups[0] = "other"
	c.Settings.Surge.Restart.Nodes[0] = "other"
	if d.Settings.Surge.Restart.Groups[0] != "Group" || d.Settings.Surge.Restart.Nodes[0] != "Node" {
		t.Fatal("clone shared restart targets")
	}
}
