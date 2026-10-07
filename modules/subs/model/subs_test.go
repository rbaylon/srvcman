package submodel

import (
	"net/http"
	"testing"
)

func validSub() *Sub {
	return &Sub{
		FirstName:     "Ana",
		LastName:      "Cruz",
		ContactNumber: "+63 917 123 4567",
		Email:         "ana.cruz@example.com",
		FramedIp:      "192.0.2.10",
	}
}

func TestBindValidatesEmail(t *testing.T) {
	good := []string{"", "ana@example.com", "a.b+tag@mail.example.co.uk", "o'neil@example.com"}
	for _, e := range good {
		s := validSub()
		s.Email = e
		if err := s.Bind(&http.Request{}); err != nil {
			t.Errorf("Bind with email %q = %v, want nil", e, err)
		}
	}
	bad := []string{
		"ana", "ana@", "@example.com", "ana@example", "ana@@example.com",
		"ana smith@example.com", "ana@exam ple.com", ".ana@example.com", "ana.@example.com",
		"ana..b@example.com", "ana@-example.com", "ana@example.com\r\nBcc: x@y.com",
		"\"ana\"@example.com", "<ana@example.com>",
	}
	for _, e := range bad {
		s := validSub()
		s.Email = e
		if err := s.Bind(&http.Request{}); err == nil {
			t.Errorf("Bind with email %q = nil, want an error", e)
		}
	}
}

func TestRequireContactDetails(t *testing.T) {
	if err := validSub().RequireContactDetails(); err != nil {
		t.Fatalf("complete sub rejected: %v", err)
	}
	blank := map[string]func(*Sub){
		"first_name":     func(s *Sub) { s.FirstName = "" },
		"last_name":      func(s *Sub) { s.LastName = "  " },
		"framed_ip":      func(s *Sub) { s.FramedIp = "" },
		"contact_number": func(s *Sub) { s.ContactNumber = "" },
		"email":          func(s *Sub) { s.Email = "" },
	}
	for field, blankIt := range blank {
		s := validSub()
		blankIt(s)
		err := s.RequireContactDetails()
		if err == nil {
			t.Errorf("blank %s accepted, want an error", field)
		} else if got := err.Error(); got != field+" is required" {
			t.Errorf("blank %s: error %q, want it to name the field", field, got)
		}
	}
}

// Subscribers created before email existed have none. Bind (which also runs
// on every update) must keep accepting them; only creation requires it.
func TestBindAcceptsLegacySubWithoutEmail(t *testing.T) {
	s := validSub()
	s.Email = ""
	if err := s.Bind(&http.Request{}); err != nil {
		t.Errorf("update of a sub with no email rejected: %v", err)
	}
}
