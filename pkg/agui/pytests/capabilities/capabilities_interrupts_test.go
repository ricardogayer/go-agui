package capabilities_test

// Port of sdks/python/tests/test_capabilities_interrupts.py.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// Python: tests/test_capabilities_interrupts.py::HumanInTheLoopCapabilitiesInterruptsTest::test_interrupts_flag
func TestPyHumanInTheLoopCapabilitiesInterruptsTest_TestInterruptsFlag(t *testing.T) {
	c := agui.HumanInTheLoopCapabilities{Interrupts: b(true)}
	if !agui.Deref(c.Interrupts) {
		t.Fatal("interrupts not true")
	}
}

// Python: tests/test_capabilities_interrupts.py::HumanInTheLoopCapabilitiesInterruptsTest::test_approve_with_edits_flag
func TestPyHumanInTheLoopCapabilitiesInterruptsTest_TestApproveWithEditsFlag(t *testing.T) {
	c := agui.HumanInTheLoopCapabilities{ApproveWithEdits: b(true)}
	if !agui.Deref(c.ApproveWithEdits) {
		t.Fatal("approveWithEdits not true")
	}
}

// Python: tests/test_capabilities_interrupts.py::HumanInTheLoopCapabilitiesInterruptsTest::test_camel_case_alias
func TestPyHumanInTheLoopCapabilitiesInterruptsTest_TestCamelCaseAlias(t *testing.T) {
	d := wire(t, agui.HumanInTheLoopCapabilities{ApproveWithEdits: b(true)})
	assertHasKey(t, d, "approveWithEdits")
	assertNoKey(t, d, "approve_with_edits")
}

// Python: tests/test_capabilities_interrupts.py::HumanInTheLoopCapabilitiesInterruptsTest::test_parse_from_camel_case
func TestPyHumanInTheLoopCapabilitiesInterruptsTest_TestParseFromCamelCase(t *testing.T) {
	var c agui.HumanInTheLoopCapabilities
	mustUnmarshal(t, `{"interrupts": true, "approveWithEdits": true}`, &c)
	if !agui.Deref(c.Interrupts) || !agui.Deref(c.ApproveWithEdits) {
		t.Fatalf("flags not parsed: %+v", c)
	}
}
