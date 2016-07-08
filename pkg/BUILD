package(default_visibility = ["//visibility:public"])

load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "awscontroller/instances",
    srcs = glob(
            include = ["awscontroller/instances/*.go"],
            exclude = ["awscontroller/instances/*_test.go"],
    ),
    deps = [
      ":kope",
      ":kope/kopeaws",
      "//vendor/github.com/golang/glog:go_default_library",
      "//vendor/github.com/spf13/pflag:go_default_library",
      "//vendor/github.com/aws/aws-sdk-go:aws",
      "//vendor/github.com/aws/aws-sdk-go:service/ec2",
      "//vendor/k8s.io/kubernetes:pkg/util/runtime",
      "//vendor/k8s.io/kubernetes:pkg/util/wait",
    ],
)

go_library(
    name = "kope",
    srcs = glob(
            include = ["kope/*.go"],
            exclude = ["kope/*_test.go"],
    ),
    deps = [
      "//vendor/github.com/golang/glog:go_default_library",
    ],
)

go_library(
    name = "kope/kopeaws",
    srcs = glob(
            include = ["kope/kopeaws/*.go"],
            exclude = ["kope/kopeaws/*_test.go"],
    ),
    deps = [
      ":kope",
      ":kope/utils",
      "//vendor/github.com/golang/glog:go_default_library",
      "//vendor/github.com/aws/aws-sdk-go:aws",
      "//vendor/github.com/aws/aws-sdk-go:aws/ec2metadata",
      "//vendor/github.com/aws/aws-sdk-go:aws/request",
      "//vendor/github.com/aws/aws-sdk-go:aws/session",
      "//vendor/github.com/aws/aws-sdk-go:service/ec2",
      "//vendor/github.com/aws/aws-sdk-go:service/route53",
    ],
)

go_library(
    name = "kope/utils",
    srcs = glob(
            include = ["kope/utils/*.go"],
            exclude = ["kope/utils/*_test.go"],
    ),
    deps = [
    ],
)
