# API review for v0.1.0 (M6.3)

Every exported identifier of `handlers`, `handlers/handlerstest`,
`handlers/authguard` and `handlers/openapi`, as `go doc -all` lists them on
2026-11-18, against [interface.md](interface.md) §11 and the rules it serves;
every generated identifier of §10 against the golden files; and every command
and flag of §6. "Checked" names a test or golden file that pins it down.

## `handlers` (§11.1–§11.3, §11.7)

### Routes and context (§11.1)

| Identifier | Interface | Checked |
|---|---|---|
| `Surface`, `HTML`, `JSON`, `Surface.String` | `"html"`, `"json"`; panics on unknown (§12.2) | `TestSurfaceAndAuthString` |
| `Auth`, `Public`, `Session`, `Permission`, `Auth.String` | `"public"`, `"session"`, `"permission"`; panics on unknown (§12.2) | `TestSurfaceAndAuthString` |
| `Route` (`Key`, `Method`, `Prefix`, `Pattern`, `Surface`, `Auth`, `Permission`, `Label`, `View`, `Searchable`, `Entity`, `Operation`, `Params`) | 13 fields as §11.1 | `TestFromAndWithRoute`, `TestMountChainAndRouting`; `testdata/app` `TestAdminRoutesHTTPAndLabels`, `TestAPIRoutesADR0003` |
| `Param` (`Name`, `Rest`, `Label`) | 3 fields as §11.1 | `TestParamHelpers`, `TestNewTable`; `testdata/app` `TestAdminRoutesHTTPAndLabels` |
| `From`, `WithRoute` | attaches/reads `Route` in context before guard (§12.1) | `TestFromAndWithRoute`, `TestMountChainAndRouting`; `testdata/app` `TestAdminRoutesHTTPAndLabels`, `TestAPIRoutesADR0003` |

### Guards, options and errors (§11.2, §12.3)

| Identifier | Interface | Checked |
|---|---|---|
| `Guard` (`Prepare`, `Check`, `Admit`) | as §11.2 | `TestMountChainAndRouting`, `TestSearch`; `authguard` `TestPrepareAndMount`, `TestMountAndCheckWithAuthtest`, `TestAdmitResultsMemoizationAndErrors`; `handlerstest` `TestGuardPrepareCheckAdmit` |
| `Option`, `WithErrors`, `WithMaxBody`, `WithMiddleware` | default 1 MiB (`0` disables, negative panics); middleware order after guard | `TestMountPanics`, `TestMountChainAndRouting`, `TestMountHandleHTTPAndWriter` |
| `ErrorWriter`, `WriteText`, `WriteJSON`, `Status` | `413` on `ErrTooLarge`/`*http.MaxBytesError`; `>= 500` logged via `slog.Default` without leaking `Error()` | `TestStatus`, `TestWriteTextAndWriteJSON`, `TestMountChainAndRouting` |
| `ErrBadParam` | `errors.handlers.bad_param` (`errors.Invalid`), localized param label | `TestStatus`, `TestParamHelpers`; `testdata/app` `TestAdminRoutesHTTPAndLabels`; `examples/shop` `TestAdminPagesAndAuth` |
| `ErrNotFound` | `errors.handlers.not_found` (`errors.NotFound`), catch-all under prefix | `TestStatus`, `TestWriteTextAndWriteJSON`, `TestMountChainAndRouting`; `testdata/app` `TestAdminRoutesHTTPAndLabels` |
| `ErrTooLarge` | `errors.handlers.too_large` (`errors.Invalid`, status 413) | `TestStatus`; `openapi` `TestOpenAPIMountBehaviour` |
| `ErrInvalidRequest` | `errors.handlers.invalid_request` (`errors.Invalid`) | `TestStatus`, `TestMountChainAndRouting`; `openapi` `TestOpenAPIMountBehaviour` |
| `ErrInvalidField` | `errors.handlers.invalid_field` (`errors.Invalid`), param name or JSON pointer | `TestStatus`, `TestWriteTextAndWriteJSON`, `TestMountHandleHTTPAndWriter`; `openapi` `TestOpenAPIMountBehaviour`; `examples/shop` `TestAPISurface` |

### For generated code (§11.3)

| Identifier | Interface | Checked |
|---|---|---|
| `Mount`, `NewMount`, `Mount.Handle`, `Mount.Done` | fail-closed panics, chain order, `/{$}` root, catch-all under prefix (§12.1–§12.5) | `TestMountPanics`, `TestMountChainAndRouting` |
| `Mount.HandleHTTP`, `Mount.Writer` | plumbing for `openapi` (amendment A1) | `TestMountHandleHTTPAndWriter`; `openapi` `TestOpenAPIMountBehaviour` |
| `ParamString`, `ParamInt`, `ParamUint`, `ParamBool`, `ParamText` | binding and `ErrBadParam.With(p.Label)` | `TestParamHelpers`; `testdata/app` `TestAdminRoutesHTTPAndLabels` |
| `Segment`, `Rest`, `Text` | path escaping; `Text` panics on `MarshalText` error | `TestSegmentRestText`; `testdata/app` `TestAdminRoutesHTTPAndLabels` |

### Search (§11.7, §14)

| Identifier | Interface | Checked |
|---|---|---|
| `Table`, `NewTable`, `Table.ByKey`, `Table.Routes` | panics on duplicate key; `Routes` returns a copy in order | `TestNewTable` |
| `Query` (`Text`, `Only`, `Limit`), `Hit` (`Route`, `Text`), `Table.Search` | one `g.Admit(r)`, one `tr.Search("route.")`, case-insensitive on text/source, key order | `TestSearch`, `BenchmarkSearch`; `examples/shop` `TestSearch` |

## `handlers/handlerstest` (§11.6)

| Identifier | Interface | Checked |
|---|---|---|
| `Guard` (`Session`, `Granted`, `Prepare`, `Check`, `Admit`) | fake guard; panics on unknown `Auth` | `TestGuardPrepareCheckAdmit`; `handlers` `TestSearch`; `testdata/app` `TestAdminRoutesHTTPAndLabels`, `TestAPIRoutesADR0003`; `openapi` `TestOpenAPIMountBehaviour` |
| `ErrNoSession`, `ErrForbidden` | `errors.handlerstest.no_session` (401), `errors.handlerstest.forbidden` (403) | `TestGuardErrorsAndStatuses`, `TestGuardPrepareCheckAdmit` |
| `AssertPublic` | fails unless public keys match; panics on unknown `Auth` | `TestAssertPublic`; `testdata/app` `TestPublicRoutes`; `examples/shop` `TestPublicRoutes` |

## `handlers/authguard` (§11.4)

| Identifier | Interface | Checked |
|---|---|---|
| `New` | panics on nil `a` or `ac`; `Prepare` refuses undeclared permissions; `Check` delegates to `a.Check`; `Admit` memoizes lazily per permission | `TestNewPanicsOnNil`, `TestPrepareAndMount`, `TestMountAndCheckWithAuthtest`, `TestAdmitResultsMemoizationAndErrors`; `testdata/app` `TestAdminRoutesWithAuthguard`; `examples/shop` `TestAdminPagesAndAuth`, `TestAPISurface`, `TestSearch` |

## `handlers/openapi` (§11.5)

| Identifier | Interface | Checked |
|---|---|---|
| `Mount` | panics on nil `mux`, `spec`, `register`, or missing route registration | `TestMountPanics`, `TestOpenAPIMountBehaviour`; `testdata/app` `TestAPIRoutesADR0003`; `examples/shop` `TestAPISurface` |
| `Router`, `Router.HandleFunc`, `Router.ServeHTTP`, `Router.RequestError`, `Router.WriteError` | `{$}` root, kin-openapi validation, value stripping, multipart streaming, debug log in `RequestError` | `TestMountPanics`, `TestOpenAPIMountBehaviour`; `testdata/app` `TestAPIRoutesADR0003`; `examples/shop` `TestAPISurface` |

## Generated Go (§10, §13)

| Generated symbol | Interface | Checked |
|---|---|---|
| `Prefix`, `Routes`, `ByKey` | §10.1 | `TestGeneratorsGolden` (`testdata/generators/{full,api,empty,strings_only,unverified}`); `testdata/app` `TestAdminRoutesHTTPAndLabels`, `TestPublicRoutes` |
| URL builders (`<RouteName>(...) string`) | §10.1 (`Segment`, `Rest`, `Text`, `strconv`, `p0`/`p1` parameter names) | `TestGeneratorsGolden` (`testdata/generators/{full,api,strings_only,unverified}`); `testdata/app` `TestAdminRoutesHTTPAndLabels`, `TestAPIRoutesADR0003` |
| `Handler` interface, `Register` | §10.2 (`surface: html`) | `TestGeneratorsGolden` (`testdata/generators/{full,empty,strings_only,unverified}`); `testdata/app` `TestAdminRoutesHTTPAndLabels`, `TestAdminRoutesWithAuthguard` |
| `Mount` | §10.3 (`surface: json`) | `TestGeneratorsGolden` (`testdata/generators/api`); `testdata/app` `TestAPIRoutesADR0003`; `examples/shop` `TestAPISurface` |
| `Perm<Key>` | §10.4 (`access.Permission`, optional `auth.Unverified`; single-package rule §9) | `TestGeneratorsGolden` (`testdata/generators/{full,api,unverified}`), `TestPermissionsTwoFilesError`, `TestPermissionsUnknownLevelError`; `testdata/app` `TestAdminRoutesWithAuthguard`, `TestAPIRoutesADR0003` |
| `L10n<RouteName>`, `L10n<RouteName><Param>`, `L10n` | §9, §13.1 (`i18n` `labels` generator) | `TestGeneratorsGolden` (passes `i18n` `labels` generator); `testdata/app` `TestAdminRoutesHTTPAndLabels` |

## Commands and flags (§6)

| Command | Flags | Checked |
|---|---|---|
| `gen <package dir>` | `-out`, `-package`, `-prefix`, `-check`, `-strict`, `-baseline` | `cmd/handlers` `TestGenGolden`, `TestGenFlagsAndErrors`, `TestGenBaseline`; `cmd/testdata/genxapp` `TestGenerated` |
| `openapi <spec>` | `-out`, `-package`, `-key-prefix`, `-check`, `-strict`, `-baseline` | `cmd/handlers` `TestOpenAPIGolden`, `TestOpenAPI72Rows`, `TestOpenAPIFlagsAndBaselineAndVet` |
| `vet <package dirs or patterns>` | `-strict`, `-baseline`, `-hook` | `cmd/handlers` `TestHandlersVetEndToEnd`, `TestHandlersVetHook`, `TestVetAcrossPackages`, `TestOpenAPIFlagsAndBaselineAndVet`, `TestPluginAndExtensionHooks` |
| `lsp` | none | `cmd/handlers` `TestHandlersSubcommands`; `cmd/internal/lsp` `TestInitializeNegotiationAndPositions`, `TestShutdownExitAndMethodNotFound`, `TestDiagnosticsOpenChangeCloseAndDisappear`, `TestTypePassOnSaveAndStaleDrop`, `TestCompletionHoverAndDefinition`, `TestFileCacheMtimeAndBuildConstraints`, `TestPortableFileURIs` |

## Differences

None beyond amendment A1 (`Mount.HandleHTTP` and `Mount.Writer`, already added
to `interface.md` §11.3 and §17).

Notes:

1. **Exported methods on `handlerstest.Guard`:** `Prepare`, `Check`, and
   `Admit` implement `handlers.Guard`; §11.6 names `Prepare` and `Check` in
   prose without repeating the method signatures in the struct block.
2. **Pre-tag `replace` directives:** `authguard/go.mod`, `openapi/go.mod`,
   and `cmd/go.mod` require `github.com/epistemer/handlers` via `replace => ../`
   until the root module is tagged `v0.1.0`, at which point the nested
   modules drop the `replace` and pin `v0.1.0`.
