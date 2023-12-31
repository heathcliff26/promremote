#!/bin/bash

set -e

script_dir="$(dirname "${BASH_SOURCE[0]}" | xargs realpath)/.."

OUT_DIR="${script_dir}/out"

if [ ! -d "${OUT_DIR}" ]; then
    mkdir "${OUT_DIR}"
fi

pushd "${script_dir}" >/dev/null

git archive -o "${OUT_DIR}/source.zip" HEAD

popd >/dev/null
