# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.1.0 (2026-07-14)


### Features

* **bindings:** add String methods for various enums in DirectML, WinML, HTML Help, Rights Management, XML, and AllJoyn ([6baaaa7](https://github.com/deploymenttheory/go-bindings-win32/commit/6baaaa7926b2add14963185e34c9fb17536fd93c))
* **bindings:** update COM method signatures to use **win32.IUnknown for out parameters ([df1ae5b](https://github.com/deploymenttheory/go-bindings-win32/commit/df1ae5bf01736bc530c667220940587127fd4f96))
* **bindings:** update COM method signatures to use **win32.IUnknown for output parameters ([a958fc8](https://github.com/deploymenttheory/go-bindings-win32/commit/a958fc894b739b0bec6f6ee7a9be9d4c227059fe))
* **com:** M3 COM vtable pipeline ([f87b40d](https://github.com/deploymenttheory/go-bindings-win32/commit/f87b40d899fdcca10326be06b4a4e4196b67d7be))
* **emit:** collapse [MemorySize] byte buffers to []byte and extend slice collapse to COM methods ([d3b5ed2](https://github.com/deploymenttheory/go-bindings-win32/commit/d3b5ed23503a946f147cb75a7dad78175e0b558c))
* **emit:** collapse [MemorySize] byte buffers to []byte and extend slice collapse to COM methods ([8ac93db](https://github.com/deploymenttheory/go-bindings-win32/commit/8ac93dbdbd884c78975a6e0cf141e16e077ecb1f))
* **emit:** emit packed structs as opaque named blobs instead of skip ([5d8ca30](https://github.com/deploymenttheory/go-bindings-win32/commit/5d8ca30bd2411490368b5d22d09cf07cabf5cc8d))
* **emit:** emit packed structs as opaque named blobs instead of skipping ([bf6e9ef](https://github.com/deploymenttheory/go-bindings-win32/commit/bf6e9ef03abb16fa610f946c3097ba25d9b0b617))
* idiomatic COM wrappers + arm64 arch tags (M4 COM, M5) ([6eb06c4](https://github.com/deploymenttheory/go-bindings-win32/commit/6eb06c45525eef05116dbc8361a02027211ca01c))
* **idiomatic:** collapse array+count params into Go slices (M6) ([f0956a8](https://github.com/deploymenttheory/go-bindings-win32/commit/f0956a85f7d6bb876090e66ccbf10d58ab6d834f))
* **idiomatic:** COM interface params use wrapper types (M6) ([ef3a4a0](https://github.com/deploymenttheory/go-bindings-win32/commit/ef3a4a054b97b6b408ae3d8b26568906bdce1275))
* **idiomatic:** elevate [out,retval] params to return values (M6) ([b9c07f4](https://github.com/deploymenttheory/go-bindings-win32/commit/b9c07f40c6c5755cb492b4b44860265b23709ea6))
* **idiomatic:** handle RAII closers from [RAIIFree] (M6) ([8e3eac1](https://github.com/deploymenttheory/go-bindings-win32/commit/8e3eac18b84b3344659e42755443811beda7610b))
* **idiomatic:** M4 idiomatic function tier ([007d010](https://github.com/deploymenttheory/go-bindings-win32/commit/007d010c5f1dc8b45e6e4b8be77c7710a9f7c19f))
* Refactor/collapse idiomatic into bindings ([18df21d](https://github.com/deploymenttheory/go-bindings-win32/commit/18df21d3e085429a5a0db14033171d1fca7d2e25))
* self-contained idiomatic layer + one-command both-tier regen + example ([b8d2353](https://github.com/deploymenttheory/go-bindings-win32/commit/b8d2353d067bee89f87eef7df4654961e9ab0def))
* Win32 bindings generator — winmd reader, raw emitter, QA gates (M0–M2) ([02057de](https://github.com/deploymenttheory/go-bindings-win32/commit/02057dee59238663445601ec26bf0fd1afa2b9af))


### Bug Fixes

* **ingest:** nested anonymous unions/structs corrupted struct layout ([d95b02d](https://github.com/deploymenttheory/go-bindings-win32/commit/d95b02de26c697d48b1375c82080feaba753acac))
