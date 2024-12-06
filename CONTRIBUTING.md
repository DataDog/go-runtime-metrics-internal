# Contributing

## Tagging Versions

Do not tag new versions for this module while it's still v0.

Users of dd-trace-go frequently run `go get -u ./...` to upgrade their application dependencies, and tagging a new version of this module will cause those users to upgrade `go-runtime-metrics-internal`, which may contain breaking changes.

Instead, simply run `go get github.com/DataDog/go-runtime-metrics-internal@<commit>` in dd-trace-go to upgrade it to the latest version of this module.

For more details see [this comment](https://github.com/DataDog/go-runtime-metrics-internal/issues/10#issuecomment-2522535398).

Alternatively we could release this module as v1, and tag new major versions when we do breaking changes. But it seems like this would be more work, so pseudo-versions should be okay for now.