# Proxy Start

<!-- impeccable:product-schema 1 -->

## Platform

adaptive

Windows x64 and macOS arm64 / amd64 native desktop utility. Linux is outside scope.

## Stack

Go and MyGO native controls. No WebView or web frontend.

## Users

Chinese-speaking users who already run a local HTTP proxy, commonly Clash, and want to start Codex or a shell with it.

## Product Purpose

Two primary capabilities: manage the local Codex desktop lifecycle and open an independent proxy console. The compact 460x420 light window has a centered status circle, two explicit start/restart and stop buttons, and a top-right settings gear. The circle refreshes discovery and status; it never starts, stops or restarts Codex. Stop is disabled when not running. Refresh uses a smooth spinning arc with fade-in/out, stable text, a minimum visible duration and reduced-motion support. Existing processes change only after the user clicks a lifecycle button; restart validates the new proxy plan before stopping.

The shared local HTTP proxy port defaults to 7897 at 127.0.0.1. The gear dialog edits the port and opens a console using the entered port. No inline brand lockup, proxy evidence or explanatory boilerplate. Proxy observations remain available through diagnostics. The interface is always light; no theme selector exists.

## Capabilities and Constraints

Windows uses package identity; macOS validates the Codex bundle identifier. Refresh running state every three seconds. Read main-process argv for explicit proxy evidence, skip Chromium children, and cache evidence by process identity. An absent proxy argument is not proof of direct traffic. A successful launch or listening port is not verified target connectivity.

The console uses PowerShell on Windows and Terminal on macOS. Proxy environment variables apply only to the new shell and its children. Clear inherited NO_PROXY values. No global environment, Clash, TUN or system routing changes. No other application adapters, multiple profiles, authentication, domain bypass, interception engine, helper, service or driver.

Settings v2 persists only the shared port. Read compatible v1 local HTTP / auto ports in memory and back up original bytes before the first v2 save. Ignore legacy themes. Preserve existing credentials and installed system components.

Close hides to tray; quitting leaves user processes alive. macOS cross-compilation is supported; signing, notarization and real-device GUI, Terminal and Codex network acceptance remain unverified on Windows.

## Product Principles

- Status is visible immediately; uncertainty stays explicit.
- Native controls, concise Chinese copy, one blue accent on light neutral surfaces.
- Defaults remove setup steps; existing user processes remain untouched.
