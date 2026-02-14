# Component 1: Project Skeleton & TUI Framework

## Context

This is the foundational component of Mathiz — an AI-native terminal app that helps children (grades 3-5) build math mastery. This spec defines the Go project structure, Cobra CLI, Bubble Tea TUI architecture, screen navigation, shared UI primitives, theming, and the Home screen. Every subsequent component builds on this skeleton.

---

## 1. Project Directory Layout

```
mathiz/
├── main.go                          # Entry point, Cobra root command
├── go.mod
├── go.sum
├── cmd/
│   ├── play.go                      # `mathiz play` subcommand
│   ├── reset.go                     # `mathiz reset` subcommand
│   └── stats.go                     # `mathiz stats` subcommand
├── internal/
│   ├── app/
│   │   └── app.go                   # Root Bubble Tea model (AppModel)
│   ├── router/
│   │   └── router.go                # Stack-based screen router
│   ├── screen/
│   │   └── screen.go                # Screen interface definition
│   ├── ui/
│   │   ├── theme/
│   │   │   └── theme.go             # Color palette & Lip Gloss styles
│   │   ├── layout/
│   │   │   └── layout.go            # Header, footer, frame composition
│   │   └── components/
│   │       ├── textinput.go          # Answer text input
│   │       ├── menu.go               # Navigation menu (list)
│   │       ├── progress.go           # Progress bar
│   │       ├── button.go             # Styled button
│   │       └── multichoice.go        # Multiple-choice selector
│   └── screens/
│       └── home/
│           ├── home.go               # Home screen model
│           └── mascot.go             # Robot mascot ASCII art
└── specs/
    ├── README.md
    └── 01-skeleton.md
```

Screens added by later components (session, summary, skillmap, gemvault, history, settings) will each get their own package under `internal/screens/`.

---

## 2. Go Module & Dependencies

**Module name:** `github.com/mathiz-ai/mathiz`

| Dependency | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/charmbracelet/bubbletea/v2` | TUI framework (Elm architecture) |
| `github.com/charmbracelet/lipgloss/v2` | Terminal styling |
| `github.com/charmbracelet/bubbles/v2` | Pre-built TUI components |
| `github.com/charmbracelet/huh` | Form components |

> **Note:** ent, sqlite, and LLM dependencies are added in later components. Component 1 has no persistence.

---

## 3. Cobra CLI Structure

### Root Command (`main.go`)

```
mathiz — AI math tutor for kids

Usage:
  mathiz [command]

Available Commands:
  play        Start a practice session (default)
  reset       Reset learner data
  stats       Show learning statistics

Flags:
  -h, --help      help for mathiz
  -v, --version   version for mathiz
```

**Behavior:**
- `mathiz` with no subcommand launches the TUI (same as `mathiz play`)
- Root command's `Run` function starts Bubble Tea with `AppModel`

### Subcommands

| Command | File | Behavior |
|---|---|---|
| `mathiz play` | `cmd/play.go` | Launches the TUI app (identical to bare `mathiz`) |
| `mathiz reset` | `cmd/reset.go` | Prompts for confirmation, resets local DB. Prints confirmation. (Stub in Component 1 — prints "Not yet implemented") |
| `mathiz stats` | `cmd/stats.go` | Prints learner stats to stdout. (Stub in Component 1 — prints "Not yet implemented") |

---

## 4. Bubble Tea Architecture

### AppModel (`internal/app/app.go`)

The root Bubble Tea model owns the router and composes the final view.

```go
type AppModel struct {
    router     *router.Router
    width      int
    height     int
}
```

**Message flow:**
1. `AppModel.Update()` receives all messages
2. Global messages (quit, resize) are handled at this level
3. All other messages are forwarded to the active screen via `router.Update(msg)`
4. `AppModel.View()` composes: `header + activeScreen.View() + footer`

**Global key bindings handled by AppModel:**
- `Ctrl+C` → quit
- `Esc` → pop screen (if stack depth > 1)
- Window resize → update dimensions, propagate to active screen

### Message Types

```go
// Navigation messages (internal/router/)
type PushScreenMsg struct { Screen screen.Screen }
type PopScreenMsg struct{}

// App-level messages (internal/app/)
type QuitMsg struct{}
```

Screens request navigation by returning `PushScreenMsg` or `PopScreenMsg` commands from their `Update()`. They never directly manipulate the router.

---

## 5. Screen Interface & Router

### Screen Interface (`internal/screen/screen.go`)

```go
type Screen interface {
    // Init returns an initial command when the screen is first created
    Init() tea.Cmd

    // Update handles messages and returns updated screen + command
    Update(msg tea.Msg) (Screen, tea.Cmd)

    // View renders the screen content (excluding header/footer)
    View(width, height int) string

    // Title returns the screen name for the header
    Title() string
}
```

- `View` receives available `width` and `height` (after subtracting header/footer) so each screen can render responsively.
- Screens are stateful — they hold their own Bubble Tea sub-models (inputs, lists, etc.)

### Router (`internal/router/router.go`)

```go
type Router struct {
    stack []screen.Screen
}

func (r *Router) Push(s screen.Screen) tea.Cmd
func (r *Router) Pop() tea.Cmd
func (r *Router) Active() screen.Screen
func (r *Router) Depth() int
func (r *Router) Update(msg tea.Msg) tea.Cmd
func (r *Router) View(width, height int) string
```

**Stack behavior:**
- App starts with Home screen on the stack (depth = 1)
- `Push` adds a screen on top, calls `Init()` on the new screen
- `Pop` removes the top screen; if depth would become 0, it's a no-op (Home is always at the bottom)
- `Active()` returns the top of the stack

---

## 6. Theme & Styling

### Color Palette

A single built-in colorful, kid-friendly theme. Bright but not garish.

| Role | Color | Hex | Usage |
|---|---|---|---|
| **Primary** | Vivid Purple | `#8B5CF6` | Titles, selected items, active borders |
| **Secondary** | Teal | `#14B8A6` | Stats, progress bars, accents |
| **Accent** | Orange | `#F97316` | Gems, rewards, highlights |
| **Success** | Green | `#22C55E` | Correct answers, mastery indicators |
| **Error** | Rose | `#F43F5E` | Wrong answers, warnings |
| **Text** | White | `#F8FAFC` | Primary text |
| **TextDim** | Slate | `#94A3B8` | Secondary text, hints, placeholders |
| **BgDark** | Deep Navy | `#0F172A` | App background (for terminals that support it) |
| **BgCard** | Dark Slate | `#1E293B` | Card/panel backgrounds |
| **Border** | Slate | `#334155` | Borders, dividers |

### Lip Gloss Styles (`internal/ui/theme/theme.go`)

```go
var (
    // Typography
    Title    lipgloss.Style  // Bold, Primary color, centered
    Subtitle lipgloss.Style  // TextDim, centered
    Body     lipgloss.Style  // Text color, normal weight
    Hint     lipgloss.Style  // TextDim, italic

    // Layout
    Header lipgloss.Style    // BgCard background, padded, full-width
    Footer lipgloss.Style    // BgCard background, padded, full-width
    Card   lipgloss.Style    // BgCard bg, Border border, rounded, padded

    // States
    Selected   lipgloss.Style  // Primary color, bold, with "▸" prefix
    Unselected lipgloss.Style  // Text color, with " " prefix
    Correct    lipgloss.Style  // Success color, bold
    Incorrect  lipgloss.Style  // Error color, bold

    // Components
    ProgressFilled lipgloss.Style  // Secondary bg
    ProgressEmpty  lipgloss.Style  // Border bg
    ButtonActive   lipgloss.Style  // Primary bg, bold, padded
    ButtonInactive lipgloss.Style  // BgCard bg, Border border
)
```

---

## 7. Responsive Layout System

### Frame Composition (`internal/ui/layout/layout.go`)

Every frame is composed as:

```
┌─────────────────────────── full terminal width ──────────────────────────┐
│ HEADER (3 lines)                                                         │
│──────────────────────────────────────────────────────────────────────────│
│                                                                          │
│ CONTENT AREA (terminal height - 6 lines)                                 │
│ (passed to active screen's View)                                         │
│                                                                          │
│──────────────────────────────────────────────────────────────────────────│
│ FOOTER (3 lines)                                                         │
└──────────────────────────────────────────────────────────────────────────┘
```

### Header (3 lines)

```
╭──────────────────────────────────────────────────────────────────╮
│  🤖 Mathiz  │  Home                    💎 42   🔥 5 day  │
╰──────────────────────────────────────────────────────────────────╯
```

- Left: App logo + mascot emoji
- Center: Current screen title (from `Screen.Title()`)
- Right: Gem count + streak counter
- In Component 1, gem count = 0, streak = 0 (hardcoded; wired up in later components)

### Footer (3 lines)

```
╭──────────────────────────────────────────────────────────────────╮
│  ↑↓ Navigate   Enter Select   Esc Back   Ctrl+C Quit    │
╰──────────────────────────────────────────────────────────────────╯
```

- Shows contextual key hints relevant to the active screen
- Each screen can provide its own footer hints via an optional method, otherwise defaults are shown

### Responsive Breakpoints

| Terminal Width | Behavior |
|---|---|
| < 80 cols | Show "Terminal too small" message with required size |
| 80–99 cols | Compact layout: tighter padding, abbreviated text |
| 100+ cols | Full layout: generous padding, full text |

| Terminal Height | Behavior |
|---|---|
| < 24 rows | Show "Terminal too small" message |
| 24–29 rows | Compact: mascot hidden on home, reduced spacing |
| 30+ rows | Full layout with mascot and spacing |

### Min-Size Screen

When the terminal is too small:

```
  ┌─────────────────────────┐
  │                         │
  │   Terminal too small!   │
  │                         │
  │   Please resize to at   │
  │   least 80 x 24        │
  │                         │
  │   Current: 62 x 18     │
  │                         │
  └─────────────────────────┘
```

---

## 8. Shared UI Components

### 8.1 Text Input (`internal/ui/components/textinput.go`)

Wraps `bubbles/textinput` with Mathiz styling.

```
  Your answer: 42█
```

- Styled cursor in Primary color
- Placeholder text in TextDim
- Optional validation indicator (checkmark / X) shown after submission
- Numeric-only mode for math answers (filters non-digit input)
- Props: `Placeholder string`, `NumericOnly bool`, `MaxWidth int`

### 8.2 Menu (`internal/ui/components/menu.go`)

Vertical navigation menu used on Home and other screens.

```
  ▸ Start Practice
    Skill Map
    Gem Vault
    History
    Settings
```

- Arrow keys to move selection
- Enter to select
- Selected item shown in Primary color with `▸` prefix
- Unselected items shown in Text color with ` ` (space) prefix for alignment
- Each item has a `Label string` and an `Action func() tea.Cmd`
- Props: `Items []MenuItem`

### 8.3 Progress Bar (`internal/ui/components/progress.go`)

Horizontal progress bar for session progress, mastery display, etc.

```
  Progress  ████████░░░░░░░░  52%
```

- Filled portion in Secondary color
- Empty portion in Border color
- Optional label (left) and percentage (right)
- Adapts width to available space
- Props: `Label string`, `Percent float64`, `ShowPercent bool`, `Width int`

### 8.4 Button (`internal/ui/components/button.go`)

Styled button for actions.

```
  ╭──────────────────╮
  │  ▸ Start Practice │
  ╰──────────────────╯
```

- Active state: Primary background, bold white text
- Inactive/unfocused: BgCard background, Border border
- Props: `Label string`, `Active bool`, `OnPress func() tea.Cmd`

### 8.5 Multiple Choice (`internal/ui/components/multichoice.go`)

For multiple-choice math questions (used by session screen in later components).

```
  What is 7 × 8?

    A)  54
  ▸ B)  56
    C)  58
    D)  63
```

- Arrow keys to move between options
- Enter to confirm
- Selected option highlighted in Primary
- After submission: correct answer in Success, wrong answer in Error
- Props: `Question string`, `Options []string`, `CorrectIndex int`

---

## 9. Home Screen

### Home Screen Model (`internal/screens/home/home.go`)

```go
type HomeScreen struct {
    menu components.Menu
}
```

Implements `screen.Screen`. Title returns `"Home"`.

### Layout — Full (100+ wide, 30+ tall)

```
╭──────────────────────────────────────────────────────────────────╮
│  🤖 Mathiz  │  Home                           💎 0    🔥 0 day  │
╰──────────────────────────────────────────────────────────────────╯

              ╭───────────╮
              │  ┌─────┐  │
              │  │ ◉ ◉ │  │
              │  │  ▽  │  │
              │  ├─────┤  │
              │  │ ±×÷ │  │
              │  └─────┘  │
              ╰───────────╯

         Hey there, math explorer!
        Ready to level up today? 🚀

           ▸ Start Practice
             Skill Map
             Gem Vault
             History
             Settings

╭──────────────────────────────────────────────────────────────────╮
│  ↑↓ Navigate   Enter Select   Ctrl+C Quit                       │
╰──────────────────────────────────────────────────────────────────╯
```

### Layout — Compact (80-99 wide, 24-29 tall)

Mascot is hidden to save vertical space:

```
╭────────────────────────────────────────────────────────╮
│  🤖 Mathiz  │  Home                    💎 0   🔥 0 day │
╰────────────────────────────────────────────────────────╯

        Hey there, math explorer! 🚀

           ▸ Start Practice
             Skill Map
             Gem Vault
             History
             Settings

╭────────────────────────────────────────────────────────╮
│  ↑↓ Navigate   Enter Select   Ctrl+C Quit              │
╰────────────────────────────────────────────────────────╯
```

### Menu Items

| Item | Action |
|---|---|
| Start Practice | Push Session screen (stub: show "Coming soon" placeholder screen) |
| Skill Map | Push Skill Map screen (stub) |
| Gem Vault | Push Gem Vault screen (stub) |
| History | Push History screen (stub) |
| Settings | Push Settings screen (stub) |

### Placeholder Screen

For Component 1, all menu items except navigating the menu itself lead to a simple placeholder:

```
╭──────────────────────────────────────────────────────────────────╮
│  🤖 Mathiz  │  Skill Map                      💎 0    🔥 0 day  │
╰──────────────────────────────────────────────────────────────────╯



                     🚧 Coming Soon!

               This feature is being built.
                   Check back later!



╭──────────────────────────────────────────────────────────────────╮
│  Esc Back   Ctrl+C Quit                                          │
╰──────────────────────────────────────────────────────────────────╯
```

This is a generic `PlaceholderScreen` in `internal/screens/placeholder/placeholder.go` that takes a title string.

---

## 10. Robot Mascot

### ASCII Art (`internal/screens/home/mascot.go`)

```
  ╭───────────╮
  │  ┌─────┐  │
  │  │ ◉ ◉ │  │
  │  │  ▽  │  │
  │  ├─────┤  │
  │  │ ±×÷ │  │
  │  └─────┘  │
  ╰───────────╯
```

- 8 lines tall, 15 characters wide
- Friendly face with big eyes (`◉ ◉`) and a small smile (`▽`)
- Body shows math symbols (`±×÷`)
- Rendered in Primary (purple) color using Lip Gloss
- The mascot is a static string constant — no animation in MVP

---

## 11. Key Bindings

### Global (handled by AppModel)

| Key | Action |
|---|---|
| `Ctrl+C` | Quit the app |
| `Esc` | Pop screen (go back). No-op on Home. |

### Home Screen

| Key | Action |
|---|---|
| `↑` / `k` | Move menu selection up |
| `↓` / `j` | Move menu selection down |
| `Enter` | Select menu item |

> Note: While the primary input is arrow keys, `j`/`k` are included as a minor convenience since the Bubbles list supports them by default. They won't be advertised in the footer.

### Placeholder Screen

| Key | Action |
|---|---|
| `Esc` | Go back to previous screen |

---

## 12. Key Interfaces & Types Summary

```go
// internal/screen/screen.go
type Screen interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Screen, tea.Cmd)
    View(width, height int) string
    Title() string
}

// internal/router/router.go
type Router struct { stack []screen.Screen }
// Methods: Push, Pop, Active, Depth, Update, View

// Navigation messages
type PushScreenMsg struct { Screen screen.Screen }
type PopScreenMsg struct{}

// internal/app/app.go
type AppModel struct { router *router.Router; width, height int }
// Implements tea.Model

// internal/ui/theme/theme.go
// Exported style variables (Title, Subtitle, Header, Footer, etc.)
// Exported color constants (Primary, Secondary, Accent, etc.)

// internal/ui/layout/layout.go
func RenderFrame(header, content, footer string, width, height int) string
func RenderHeader(title string, gems, streak int, width int) string
func RenderFooter(hints []KeyHint, width int) string
type KeyHint struct { Key, Description string }

// internal/ui/components/ — each component is a Bubble Tea model
// that implements Init/Update/View and can be embedded in screens
```

---

## 13. Build Order

Within Component 1, implement in this order:

1. **Go module + Cobra CLI** — `go mod init`, `main.go`, stub subcommands
2. **Theme** — color palette, Lip Gloss style definitions
3. **Screen interface + Router** — `Screen` interface, stack-based `Router`
4. **Layout** — header, footer, frame composition, responsive logic, min-size check
5. **UI Components** — text input, menu, progress bar, button, multiple choice
6. **Home screen** — mascot, greeting, menu, wired into router
7. **Placeholder screen** — generic "coming soon" screen for all stub menu items
8. **AppModel** — wire everything together, launch Bubble Tea program

---

## 14. Verification

After implementation, verify by:

1. `go build ./...` — project compiles cleanly
2. `go vet ./...` — no vet warnings
3. `./mathiz` — launches TUI, shows Home screen with mascot and menu
4. `./mathiz play` — same as bare `./mathiz`
5. `./mathiz reset` — prints "Not yet implemented"
6. `./mathiz stats` — prints "Not yet implemented"
7. Arrow keys navigate the menu, Enter pushes placeholder screen, Esc pops back
8. Ctrl+C quits from any screen
9. Resize terminal — layout adapts, going below 80x24 shows min-size message
10. All colors and styles render correctly in a dark terminal
