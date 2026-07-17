package config

import "testing"

func TestAddException(t *testing.T) {
	c := &Config{Entries: []Entry{{Pattern: "*.plshackme.com", Target: "192.168.10.11", Port: 443}}}

	// Normalises (trim/lowercase/trailing dot) and appends.
	if err := c.AddException("*.plshackme.com", "  Public.Plshackme.com.  "); err != nil {
		t.Fatalf("AddException: %v", err)
	}
	if got := c.Entries[0].Except; len(got) != 1 || got[0] != "public.plshackme.com" {
		t.Fatalf("Except = %v, want [public.plshackme.com]", got)
	}

	// Duplicate rejected.
	if err := c.AddException("*.plshackme.com", "public.plshackme.com"); err == nil {
		t.Error("AddException duplicate: want error, got nil")
	}

	// Unknown entry rejected.
	if err := c.AddException("*.nope.com", "x.nope.com"); err == nil {
		t.Error("AddException unknown entry: want error, got nil")
	}

	// Empty exception rejected.
	if err := c.AddException("*.plshackme.com", "   "); err == nil {
		t.Error("AddException empty: want error, got nil")
	}
}

func TestRemoveException(t *testing.T) {
	c := &Config{Entries: []Entry{{
		Pattern: "*.plshackme.com", Target: "192.168.10.11", Port: 443,
		Except: []string{"public.plshackme.com"},
	}}}

	removed, err := c.RemoveException("*.plshackme.com", "Public.plshackme.com")
	if err != nil || !removed {
		t.Fatalf("RemoveException = (%v, %v), want (true, nil)", removed, err)
	}
	if len(c.Entries[0].Except) != 0 {
		t.Fatalf("Except = %v, want empty", c.Entries[0].Except)
	}

	// Removing a missing exception: no error, removed=false.
	removed, err = c.RemoveException("*.plshackme.com", "gone.plshackme.com")
	if err != nil || removed {
		t.Errorf("RemoveException missing = (%v, %v), want (false, nil)", removed, err)
	}

	// Unknown entry errors.
	if _, err := c.RemoveException("*.nope.com", "x"); err == nil {
		t.Error("RemoveException unknown entry: want error, got nil")
	}
}
