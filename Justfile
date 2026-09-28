set positional-arguments

default:
  @just --list

# Use --label-filter to select tests by label expression. All other arguments are
# passed to the test suite (e.g., `just test --label-filter cnv --as system:admin`).
test *ARGV:
  #!/usr/bin/env bash
  set -euo pipefail
  ginkgo_args=()
  suite_args=()
  while (( $# )); do
    case $1 in
      --label-filter=*)
        ginkgo_args+=("$1") ;;
      --label-filter|-l)
        ginkgo_args+=("--label-filter=${2:?--label-filter requires a value}")
        shift
        ;;
      *)
        suite_args+=("$1")
        ;;
    esac
    shift
  done
  exec ginkgo -v -p --junit-report=report.xml "${ginkgo_args[@]}" -- "${suite_args[@]}"

# Run the tests and generate an HTML report.
report:
  @just test || :
  @just convert-report

# View the generated report in a browser.
view-report: report
  xdg-open report.html

convert-report:
  go run ./cmd/junit2html -o report.html report.xml

