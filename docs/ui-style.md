# UI style conventions

CiteBox keeps a warm reading palette while using compact, consistent controls for
library management and quiet surfaces for reading and editing. Warm, light, and
dark themes share the same component dimensions.

## Shared tokens

Define shared values in `web/static/css/variables.css` and reuse them in components.

| Token | Value / purpose |
| --- | --- |
| `--radius-panel` | 20px, page panels and dialogs |
| `--radius-card` | 12px, cards and nested surfaces |
| `--radius-control` | 10px, buttons and form fields |
| `--control-height` | 3rem, standard buttons and single-line fields |
| `--control-height-sm` | 2.25rem, row actions and compact toolbars |
| `--space-1/2/3/4/6/8` | 4/8/12/16/24/32px at the default root font size |
| `--text-sm/body/section/title` | Supporting text, body, section heading, page title |
| `--shadow` | Subtle surface separation |
| `--shadow-floating` | Dialogs, menus, and floating content |

`--radius` and `--radius-sm` remain aliases for existing consumers. Keep circles,
pills, selection markers, and specialized viewer geometry appropriate to their
function; do not replace those radii mechanically.

Use `--panel-strong`, `--bg-soft`, `--line`, `--ink`, and `--muted` for regular
surfaces and text. Reserve strong accent fills for the main action. Keep borders
and focus indicators visible in all themes. Ordinary cards should not lift or
zoom merely because the pointer passes over them.

## Collection pages

`web/static/css/pages/collections.css` defines the shared layout for papers,
figures, and notes:

- A short page title, contextual actions, and native expandable page help.
- A compact filter panel that retains every existing filter.
- Inline result summaries rather than nested statistic cards.
- Sans-serif section headings and lightly styled row actions.
- A segmented control for note type and a matching library view switch.

Keep serif display typography for the brand and page titles. Do not add decorative
English section labels to a localized management page. New UI copy must use the
existing locale system with matching Chinese and English keys.

Figure thumbnails contain the entire image. Badges occupy their own strip above
the image so they do not cover figure labels or axes. Full image previews use
plain surfaces without decorative gradients or rounded clipping of image corners.

## Navigation and interaction

`AppNav` measures the available desktop width and moves excess primary links into
More. It moves the existing DOM nodes, preserves each route and the active state,
and restores the original order as space becomes available. The existing mobile
menu remains in use below 721px.

Native selects retain their keyboard and platform popup behavior. Their closed
appearance matches input fields; forced-colors mode restores the platform arrow.
Keyboard focus has an outline independent of shadows, and reduced-motion mode
disables decorative transitions, including theme transitions.

Page initialization waits for locale loading before rendering dynamic labels.
Navigation is enhanced before that wait so the locale pass also translates its
generated labels.

## Verification

Run `make ci` and syntax-check every touched JavaScript file. For visual changes,
check Chinese and English, warm/light/dark themes, desktop and narrow windows,
keyboard focus, menus, selects, empty states, and populated content. Include AI,
settings, and a reading or editing dialog when shared components change.

Use isolated fixture responses for visual previews rather than modifying a user's
library. Browser previews do not replace a native macOS WebKit check when a change
depends on that runtime.
