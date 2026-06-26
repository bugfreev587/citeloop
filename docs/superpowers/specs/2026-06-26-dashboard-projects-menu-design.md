# Dashboard Projects Menu Design

## Goal

Move account-level controls out of the standalone Projects page pattern and into the Dashboard sidebar footer. Clicking the left-bottom Projects trigger opens an upward popover that manages projects and account actions.

## Approved Visual Direction

- The trigger stays at the Dashboard sidebar lower-left.
- The popover opens upward from the trigger and borrows the reference screenshot's rounded white panel, faint blue-gray border, soft shadow, pale section dividers, and light selected-row treatment.
- Text should feel medium/regular, not heavy.
- Account Settings and Admin icons are larger than row text icons but not oversized; Account Settings is slightly larger than Admin.
- The Light theme icon is enlarged to match the reference's visual rhythm.

## Menu Sections

1. Projects list: current project and other projects.
2. Account actions: Account Settings followed by Admin when the user is a platform admin.
3. Theme: Light and Dark segmented choices.
4. Logout.

## Scope

- Project work content remains in the Dashboard left navigation.
- The old footer identity link to `/projects` is replaced with a popover button.
- The standalone `/projects` route may remain as a compatibility fallback for onboarding and direct URLs, but it is no longer the Dashboard footer entry point.
- No extra resource links from the reference screenshot are added.

## Testing

Use existing file-based contract tests to require the new menu structure and removal of the old Projects page footer link. Run the web contract test suite, typecheck, and build before shipping.
