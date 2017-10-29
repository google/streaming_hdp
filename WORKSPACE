http_archive(
    name = "io_bazel_rules_go",
    url = "https://github.com/bazelbuild/rules_go/releases/download/0.6.0/rules_go-0.6.0.tar.gz",
    sha256 = "ba6feabc94a5d205013e70792accb6cce989169476668fbaf98ea9b342e13b59",
)
load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains", "go_repository")

go_repository(
  name = "com_github_phayes_freeport",
  importpath = "github.com/phayes/freeport",
  commit = "e27662a4a9d6b2083dfd7e7b5d0e30985daca925",
)

go_repository(
  name = "com_github_gorilla_websocket",
  importpath = "github.com/gorilla/websocket",
  commit = "ea4d1f681babbce9545c9c5f3d5194a789c89f5b",
)

go_rules_dependencies()
go_register_toolchains()

load("@io_bazel_rules_go//proto:def.bzl", "proto_register_toolchains")
proto_register_toolchains()

# Needed for tests
load("@io_bazel_rules_go//tests:bazel_tests.bzl", "test_environment")
test_environment()
