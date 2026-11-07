# Install the pinned Suxin CLI release.
.PHONY: cli cli.install
cli:
	go install github.com/suxinwl/GoSuxin/framework/cmd/suxin@v1.0.0
cli.install:
	suxin version
