#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
SCRIPT_ROOT="$(dirname "${SCRIPT_DIR}")"
GEN_VER=$( awk '/k8s.io\/code-generator/ { print $2 }' "${SCRIPT_ROOT}/go.mod" )
CODEGEN_PKG=${GOPATH}/pkg/mod/k8s.io/code-generator@${GEN_VER}
BOILERPLATE="${SCRIPT_ROOT}/hack/boilerplate.go.txt"

# Remove previously generated code
rm -rf "${SCRIPT_ROOT}/pkg/client/clientset/*"
rm -rf "${SCRIPT_ROOT}/pkg/client/listeners/*"
rm -rf "${SCRIPT_ROOT}/pkg/client/informers/*"
crds=(images:v1 )
for crd in "${crds[@]}"
do
  crd_path=$(tr : / <<< "$crd")
  rm -f "${SCRIPT_ROOT}/pkg/apis/${crd_path}/zz_generated.deepcopy.go"
done

# shellcheck disable=SC1091
source "${CODEGEN_PKG}/kube_codegen.sh"

# Create a symlink so that the root of the repo is inside github.com/Cdayz/k8s-image-pre-puller.
# This is required because the codegen scripts expect it.
mkdir -p "${SCRIPT_ROOT}/github.com/linkerd"
ln -s "$(realpath "${SCRIPT_ROOT}")" "${SCRIPT_ROOT}/github.com/Cdayz/k8s-image-pre-puller"

kube::codegen::gen_helpers \
    --input-pkg-root github.com/Cdayz/k8s-image-pre-puller/pkg/apis \
    --output-base "${SCRIPT_ROOT}" \
    --boilerplate "${BOILERPLATE}"

if [[ -n "${API_KNOWN_VIOLATIONS_DIR:-}" ]]; then
    report_filename="${API_KNOWN_VIOLATIONS_DIR}/codegen_violation_exceptions.list"
    if [[ "${UPDATE_API_KNOWN_VIOLATIONS:-}" == "true" ]]; then
        update_report="--update-report"
    fi
fi

kube::codegen::gen_openapi \
    --input-pkg-root github.com/Cdayz/k8s-image-pre-puller/pkg/apis \
    --output-pkg-root github.com/Cdayz/k8s-image-pre-puller/pkg\
    --output-base "${SCRIPT_ROOT}" \
    --report-filename "${report_filename:-"/dev/null"}" \
    ${update_report:+"${update_report}"} \
    --boilerplate "${BOILERPLATE}"

kube::codegen::gen_client \
    --with-watch \
    --input-pkg-root github.com/Cdayz/k8s-image-pre-puller/pkg/apis \
    --output-pkg-root github.com/Cdayz/k8s-image-pre-puller/pkg/client \
    --output-base "${SCRIPT_ROOT}" \
    --boilerplate "${BOILERPLATE}"

# Once the code has been generated, we can remove the symlink.
rm -rf "${SCRIPT_ROOT}/github.com"
