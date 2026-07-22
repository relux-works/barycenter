# BUG-260722-224lo9: windows-join-controls-expose-pane-uia-semantics

## Description
On the installed 0.1.20 Windows candidate, visible native Join controls expose stable AutomationIds but UI Automation reports ControlType.Pane, no ValuePattern or InvokePattern, and the invitation Edit as not keyboard-focusable.

## Scope
Correct the Windows Join navigation, invitation input, and Join securely accessibility providers without changing native control IDs or secure onboarding behavior. Add focused automated coverage on a packaged Windows candidate.

## Acceptance Criteria
AutomationIds 3003 and 3010 expose Button with InvokePattern; 3027 exposes Edit with ValuePattern and keyboard focus; native controls remain visible, enabled, and usable; packaged UI Automation regression test passes without entering an invitation or invoking real Join.
