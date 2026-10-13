# Accessible selection and annotation controls (#19)

The waveform displays the draft A/B selection and draggable handles. Each handle
is a labelled slider in the keyboard focus order. Arrow keys move one source
frame; Shift-arrow moves ten; Home/End move to the visible waveform range.
`aria-valuetext` reports source seconds with six decimal places. Pointer positions
are clamped to the visible range. Dragging B cannot also update A via a synthetic
click. Busy operations disable gestures and remove handles from the focus order.

These gestures edit selection fields only. **Apply A/B loop** sends the values
through the existing shared command facade; core validates the range and updates
transport. Speed does not change the selection's source-time coordinates. Exact
zoom and overview retain their existing shared waveform query paths.

Choose a saved phrase to edit its name, timing and notes with **Update selected
phrase**. Associated named-loop IDs remain stable and labels are updated together.
**Remove selected phrase** removes its region/named loops via the shared core;
a phrase referenced by a saved plan is rejected until that plan is removed or
changed. Saved section, measure and beat markers are listed with source times and can
set A or be removed. Beat markers include measure/beat numbers for saved-plan
beat grids; referenced timing is validated by the core before changes are accepted.
Project/device-save/download paths preserve the resulting annotations.

`node scripts/check-practice-waveform.mjs` exercises the actual React controls:
source-frame keyboard precision, zoomed range boundaries, coordinate clamping,
B drag/click separation and disabled state. Workspace tests retain Strict Mode
import/disposal coverage. Build/type checks remain mandatory. Full screen-reader,
touch-device and musical workflow acceptance are outstanding.
