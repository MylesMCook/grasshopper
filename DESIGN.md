---
version: alpha
name: Myles Design Foundation
description: A calm editorial foundation for adaptable web and native products.
omitted:
  - section: components
    reason: Component implementations and interaction patterns are chosen within each product.
colors:
  primary: "#282828"
  on-primary: "#ffffff"
  background: "#ffffff"
  surface: "#f7f7f5"
  foreground: "#282828"
  muted: "#595955"
  border: "#90908c"
  divider: "#e1e1de"
  link: "#125ab8"
  focus: "#125ab8"
  success: "#087d60"
  warning: "#8a5100"
  danger: "#a7282a"
typography:
  display:
    fontFamily: Newsreader
    fontSize: 64px
    fontWeight: 400
    lineHeight: 1.05
    letterSpacing: 0.005em
  heading:
    fontFamily: Newsreader
    fontSize: 36px
    fontWeight: 400
    lineHeight: 1.15
  reading:
    fontFamily: Newsreader
    fontSize: 18px
    fontWeight: 400
    lineHeight: 1.55
  body:
    fontFamily: Geist Mono
    fontSize: 16px
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: Geist Mono
    fontSize: 14px
    fontWeight: 500
    lineHeight: 1.35
  brand:
    fontFamily: Geist Mono
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.2
  code:
    fontFamily: Geist Mono
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.5
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 40px
  section: 64px
rounded:
  content: 2px
  control: 2px
---

## Overview

This is a starting design language for Myles's products, inspired by the editorial clarity of Thinking Machines. A product's task, audience, content, and platform determine which parts to use. Each project should keep the roles that fit, replace tokens that do not, and record those decisions in its own `DESIGN.md`.

## Colors

Let ink, paper, and space establish the hierarchy. Use saturated color for meaningful status and links, not as a substitute for structure. A product may replace the accent and status hues while preserving their semantic roles and readable contrast. Use the stronger border token for controls; use the quieter divider only between clearly grouped content.

## Themes

The frontmatter is the light default. The current DESIGN.md alpha format does not encode theme modes, so a project supporting dark appearance should apply these corresponding defaults before tailoring them to its content:

| Token | Dark value |
| --- | --- |
| primary | `#f5f5f2` |
| on-primary | `#171717` |
| background | `#171717` |
| surface | `#222222` |
| foreground | `#f5f5f2` |
| muted | `#b8b8b4` |
| border | `#777773` |
| divider | `#454545` |
| link | `#9cc8ff` |
| focus | `#9cc8ff` |
| success | `#89d8b8` |
| warning | `#ffd08a` |
| danger | `#ff9e9e` |

## Typography

Use Newsreader to introduce an idea or support sustained reading, including its italic face for emphasis. Use Geist Mono for body copy, navigation, forms, controls, tables, code, and short identity labels. Its fixed width takes more space, so check dense task screens and narrow viewports without shrinking text. The display role belongs on spacious entry points, not routine task screens. Keep headings few and decisive. Bundle these open fonts where the platform permits, preserve user text scaling, and provide appropriate serif and monospace fallbacks.

## Layout

Begin with one clear reading path. Keep prose in a narrow measure, but let tools and comparative data use the width they need. Group by the user's task, then separate groups with space or a rule. On small screens, preserve the primary action and its context before reducing decorative space. Dense screens may tighten spacing without shrinking text or targets.

## Elevation & Depth

Use tonal surfaces and rules for ordinary grouping. Reserve shadow and overlay treatment for content that genuinely floats above the current task. A flat layout should still make ownership, boundaries, and active state obvious.

## Shapes

Use nearly square corners for reading surfaces, buttons, fields, and dialogs. Keep one shape language within a screen.

## Components

Give each decision region one clear primary action. Make secondary actions quieter while preserving their labels and focus states. Put persistent labels on fields, explain errors near the affected control, and show selected or pending states with more than color. Use cards only when they establish a real boundary; prefer open layout for continuous reading. Make tables and charts easy to compare, with direct labels and restrained color. Motion should clarify a state change and yield to reduced-motion settings.

## Do's and Don'ts

- Do verify contrast, keyboard focus, text scaling, and narrow-screen reflow in the finished product.
- Do use imagery when it conveys content or helps explain a task.
- Don't add abstract decoration to functional screens.
- Don't copy Thinking Machines logos, illustrations, or proprietary typefaces.

## Grasshopper application

The private memory view applies this foundation directly: Geist Mono for navigation, labels, controls, metadata and status; Newsreader for headings and full memory text; the specified light/dark semantic colors; and 2px corners on controls, surfaces and dialogs. Fonts are bundled with the service. The public landing/setup pages keep their existing matching Newsreader/Geist Mono assets.

This is a routine reading/search screen, so its main heading uses the 36px heading role instead of the 64px display role. Repeated memory titles use 24px Newsreader to keep the list scannable. Memory prose uses the 18px reading role and a narrow measure. Small metadata uses 12–14px while body and controls retain their normal sizes; native targets remain at least 44px high. Filters stack on narrow screens.

Search and scope selectors form one task group. The selected scope and count precede open memory entries. Device management is secondary progressive disclosure; direct approval links still focus the matching request. Whitespace separates continuous reading, so no decorative divider token is applied to each record or section. Control borders and visible keyboard focus identify functional boundaries.
