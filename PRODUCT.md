# Proxy Start

<!-- impeccable:product-schema 1 -->

当前目标版本：0.1.10。Windows x64 是首版完整交付平台；macOS 提供 arm64 / amd64 源码与交叉编译包，原生适配仅 Codex，强制代理、实机 GUI / Codex 网络、签名和公证仍未完成验收。

## Platform

adaptive

Windows and macOS native desktop clients. Linux is outside the current scope.

## Stack

MyGO native Go UI, with explicitly injected platform adapters. No web frontend.

## Users

Chinese-speaking desktop users who already have a local or remote proxy and need to start applications through it.

## Product Purpose

Add a proxy, drag in an application, then start, stop or restart it. The initial native-adapter allowlist contains only Codex, on Windows and macOS. Other explicitly added Windows applications use per-application interception without TUN. macOS rejects unsupported proxy launches rather than silently starting them without a proxy. Proxy profiles also have a list-level console action and modal create/edit flow; operation feedback is delivered as top-right floating notices.

Proxy profiles also expose an independent proxy console. `Desktop.OpenProxyConsole` resolves the selected profile, prefers a verified HTTP result during automatic protocol detection, and delegates shell creation to the platform `application.ConsoleLauncher`. Windows opens a new PowerShell; macOS opens Terminal through a private temporary script. The console receives HTTP / HTTPS / ALL proxy variables in upper- and lower-case for the new shell and its child processes, clears NO_PROXY, URL-encodes credentials, and never changes the parent process or global system environment. Only programs that honor proxy environment variables follow the console. Opening the shell does not prove real target traffic connectivity.

## Capabilities and Constraints

HTTP and SOCKS5; default proxy and per-application override; optional credentials. Window close hides to tray. Full quit stops interception and leaves target processes alive. Proxy connectivity after full quit is not required. Do not change Clash configuration, system proxy or global routing. GUI remains unelevated; driver setup and engine control use a helper. Do not claim a connection was verified merely because a process started.

Codex native launch currently accepts unauthenticated HTTP only. macOS uses .app identities, direct process launch and Keychain credentials, without a forced-proxy engine. Mac packages are cross-compiled for arm64 and amd64; signing, notarization, real Mac GUI / Codex / console / Keychain acceptance and actual target traffic validation are not complete. CI or Windows cross-compilation does not establish those Mac results.

## Product Principles

- Visible, honest launch mode and runtime state.
- No interception of applications not explicitly added and activated.
- Cohesive responsibilities, small consumer-defined interfaces, no global service locator.
- Native desktop controls and concise Chinese copy.

## Evidence on Hand

Reference launcher: D:\codex-start-with-tun\start-codex-with-clash.ps1. Local Clash mixed proxy: 127.0.0.1:7897.
