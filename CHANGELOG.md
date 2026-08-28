# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0](https://github.com/deploymenttheory/go-bindings-win32/compare/v0.3.1...v0.4.0) (2026-08-28)


### Features

* **codegen:** [Optional] input strings become *string (nil passes NULL) ([9644899](https://github.com/deploymenttheory/go-bindings-win32/commit/9644899575c8ea15bdb72030926c10ffd5cdea1b))
* **codegen:** dispatch floats, by-value composites and struct returns via win32.Call ([c106e23](https://github.com/deploymenttheory/go-bindings-win32/commit/c106e23cd75bca9c1ffa8f81bba7b2a4d83e81c9))
* **codegen:** emit [Constant] header inlines instead of FORCEINLINE dispatch ([a568505](https://github.com/deploymenttheory/go-bindings-win32/commit/a5685054049454bde5ab13a06addcedd28e50ac4))
* **codegen:** make Close&lt;Handle&gt; a no-op for zero and invalid sentinels ([e158828](https://github.com/deploymenttheory/go-bindings-win32/commit/e158828fffd82e33dc3c3450723a99185420af81))
* **codegen:** marshal native C bool parameters and returns ([ec11553](https://github.com/deploymenttheory/go-bindings-win32/commit/ec11553f0fc5645600ab2a5817ca33ebe53c2289))
* **codegen:** struct returns — hidden result pointer for COM, r1 for flat ([e869f3a](https://github.com/deploymenttheory/go-bindings-win32/commit/e869f3a040edc9a5662dc2b2c77d23e9e3b83ab3))
* **com:** one IUnknown type plus QueryInterface[T] and Cast[T] helpers ([ea1b463](https://github.com/deploymenttheory/go-bindings-win32/commit/ea1b46347f2f1c6f604b639cf45db1075de7ec80))
* **generate:** coverage report (docs/COVERAGE.md) from the emitting run ([de057f0](https://github.com/deploymenttheory/go-bindings-win32/commit/de057f0ba4fa43a6acfe7bd779e0864e63e7e81b))
* register-aware calls, full ABI gate, and review-driven fixes ([f046700](https://github.com/deploymenttheory/go-bindings-win32/commit/f04670053b003b7be6d4b4717c48da5393daaed7))
* **runtime:** register-aware Call with per-arch assembly trampolines ([c080289](https://github.com/deploymenttheory/go-bindings-win32/commit/c080289ad05391f27556f3ce61f23618ad632fd4))
* **runtime:** typed ProcError and per-package Procs availability-probe table ([e3d2c10](https://github.com/deploymenttheory/go-bindings-win32/commit/e3d2c106126b490972f13ded87530c6f3b806b42))

## [0.3.1](https://github.com/deploymenttheory/go-bindings-win32/compare/v0.3.0...v0.3.1) (2026-08-14)


### Bug Fixes

* gofmt the repository after the go-winmd path change ([a63c453](https://github.com/deploymenttheory/go-bindings-win32/commit/a63c45361f67e5beac676f239df258914585891b))
* resolve the four open code-scanning alerts ([68dc4cc](https://github.com/deploymenttheory/go-bindings-win32/commit/68dc4cc5042984ba0348b0dffec3a3fefd6f2b5b))
* resolve the four open code-scanning alerts ([88f6f1e](https://github.com/deploymenttheory/go-bindings-win32/commit/88f6f1efad404fb3fe469c478a0026d015f0bcf2))
* restore import ordering after the go-winmd path change ([d3e21dd](https://github.com/deploymenttheory/go-bindings-win32/commit/d3e21dd129296fbe72c7937a98b1c1ec916cc451))

## [0.3.0](https://github.com/deploymenttheory/go-bindings-win32/compare/v0.2.1...v0.3.0) (2026-08-12)


### Features

* **codegen:** pass register-sized by-value structs, unlocking ConPTY ([0847514](https://github.com/deploymenttheory/go-bindings-win32/commit/08475144d7e2c852750b62ec7740af02242897ef))
* **codegen:** pass register-sized by-value structs, unlocking ConPTY ([e5f99fc](https://github.com/deploymenttheory/go-bindings-win32/commit/e5f99fce9e207f307da2ec7e0a383589c2344782))


### Bug Fixes

* update terminology from "oracle" to "trusted source" in documentation ([3b02b9d](https://github.com/deploymenttheory/go-bindings-win32/commit/3b02b9d6d09273e0cfd621c5dc924d7b86d9a39e))

## [0.2.1](https://github.com/deploymenttheory/go-bindings-win32/compare/v0.2.0...v0.2.1) (2026-07-19)


### Bug Fixes

* correct Install docs to reflect tagged releases ([d76953d](https://github.com/deploymenttheory/go-bindings-win32/commit/d76953d177000464b5036672dd012f9bd35f1969))
* correct Install docs to reflect tagged releases ([37b30aa](https://github.com/deploymenttheory/go-bindings-win32/commit/37b30aa14825b9d912012d0b9551ee9885611478))

## [0.2.0](https://github.com/deploymenttheory/go-bindings-win32/compare/v0.1.0...v0.2.0) (2026-07-16)


### Features

* extract the ECMA-335 reader to deploymenttheory/go-winmd ([9d9b04a](https://github.com/deploymenttheory/go-bindings-win32/commit/9d9b04a32830a1071fe626869ae66c382834ac06))


### Bug Fixes

* **ci:** let include:scope derive the dependabot commit scope ([f326f35](https://github.com/deploymenttheory/go-bindings-win32/commit/f326f3509fc2860b3a070400ac2959f520b8636d))
* **ci:** stop dependabot doubling the commit scope to chore(deps)(deps) ([b1d98be](https://github.com/deploymenttheory/go-bindings-win32/commit/b1d98be7e4d18112b3645f8cd8996c8607e7c666))
* heap-escape elevated out-params (stack-move hazard under reentrant callbacks) ([5bd1590](https://github.com/deploymenttheory/go-bindings-win32/commit/5bd1590ef5e291192fd958addb10b8a04e404cf9))
* heap-escape elevated out-params (stack-move hazard under reentrant callbacks) ([cd98306](https://github.com/deploymenttheory/go-bindings-win32/commit/cd98306344a35df9ab940f916fc08f3ddbeaa4a6))

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
