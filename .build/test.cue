jobs: test_all: {
	command: "test"
	all_revision: true
	github_status: true
	targets: ["//..."]
	platforms: ["@rules_go//go/toolchain:linux_amd64"]
	cpu_limit: "2000m"
	event: ["push", "pull_request"]
}
