# Use MyGo Native UI with Onevra

Status: the Onevra half is superseded by [ADR 0002](0002-mygo-native-ui-without-onevra.md). MyGo Native UI stands; Onevra does not.

GitOwl will build its macOS interface in MyGo's Native UI mode and use Onevra for native desktop controls and window layout. Here, "native UI" means Go-defined views rendered without a WebView; it does not mean SwiftUI or AppKit controls. This keeps the interface in Go and reuses Onevra's component library, with the trade-off that the interface follows MyGo's rendering and input model. See [MyGo](https://mygo.egoist.dev/) and [Onevra](https://oneo.me/zh-Hans/onevra).
