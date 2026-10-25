package agui

import (
	"fmt"
	"strconv"
	"strings"
)

// ProtocolVersion is the AG-UI protocol version this package implements. It
// is declared in-band: on RunAgentInput.protocolVersion by a consumer and on
// RUN_STARTED.protocolVersion by a producer.
const ProtocolVersion = "1.0"

// Version is a parsed MAJOR.MINOR protocol version identifier.
type Version struct {
	Major, Minor int
}

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// ParseVersion parses a MAJOR.MINOR version identifier. Components compare
// numerically, so 1.10 is newer than 1.9.
func ParseVersion(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		return Version{}, fmt.Errorf("ag-ui: cannot interpret protocol version %q (expected MAJOR.MINOR)", s)
	}
	var v [2]int
	for i, p := range parts {
		if p == "" || strings.TrimLeft(p, "0123456789") != "" || (len(p) > 1 && p[0] == '0') {
			return Version{}, fmt.Errorf("ag-ui: cannot interpret protocol version %q (expected MAJOR.MINOR)", s)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("ag-ui: cannot interpret protocol version %q: %w", s, err)
		}
		v[i] = n
	}
	return Version{Major: v[0], Minor: v[1]}, nil
}

// Compare returns -1, 0 or +1 as v is older than, equal to or newer than w.
func (v Version) Compare(w Version) int {
	switch {
	case v.Major != w.Major:
		return sign(v.Major - w.Major)
	default:
		return sign(v.Minor - w.Minor)
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// CurrentVersion is ProtocolVersion, parsed.
var CurrentVersion = Version{Major: 1, Minor: 0}

// PeerVersion classifies a peer's version declaration.
type PeerVersion int

const (
	// PeerUndeclared: no declaration — a peer from before the protocol
	// carried a version.
	PeerUndeclared PeerVersion = iota
	// PeerOlder: an older version of a line this implementation speaks.
	PeerOlder
	// PeerSame: the version this implementation speaks.
	PeerSame
	// PeerNewerMinor: a newer minor of this major line. It must be served —
	// additions are safe — with a warning.
	PeerNewerMinor
	// PeerOtherMajor: a major line this implementation does not implement. A
	// producer MAY reject it before RUN_STARTED.
	PeerOtherMajor
	// PeerUninterpretable: a declaration this implementation cannot parse.
	// Handled like a newer one: proceed, and warn.
	PeerUninterpretable
)

// ClassifyPeerVersion classifies a peer's declaration (nil when absent)
// against ProtocolVersion, returning a warning to surface when the
// specification says one should be.
func ClassifyPeerVersion(declared *string) (PeerVersion, *Warning) {
	if declared == nil {
		return PeerUndeclared, nil
	}
	v, err := ParseVersion(*declared)
	if err != nil {
		return PeerUninterpretable, &Warning{Path: "/protocolVersion", Message: fmt.Sprintf("cannot interpret peer protocol version %q; proceeding as %s", *declared, ProtocolVersion)}
	}
	switch {
	case v.Major != CurrentVersion.Major:
		if v.Major > CurrentVersion.Major {
			return PeerOtherMajor, &Warning{Path: "/protocolVersion", Message: fmt.Sprintf("peer speaks protocol %s, a major line this implementation (%s) does not implement", v, ProtocolVersion)}
		}
		return PeerOtherMajor, nil
	case v.Minor > CurrentVersion.Minor:
		return PeerNewerMinor, &Warning{Path: "/protocolVersion", Message: fmt.Sprintf("peer speaks protocol %s, newer than %s; material this version does not recognise will be stripped", v, ProtocolVersion)}
	case v.Minor < CurrentVersion.Minor:
		return PeerOlder, nil
	}
	return PeerSame, nil
}
