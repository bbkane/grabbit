#!/bin/bash

# exit the script on command errors or unset variables
# http://redsymbol.net/articles/unofficial-bash-strict-mode/
set -euo pipefail
IFS=$'\n\t'

# https://stackoverflow.com/a/246128/295807
readonly script_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "${script_dir}"

readonly tag="$1"  # should start with v
readonly message="$2"

set -x

git tag -a "${tag}" -m "${message}"
git push origin "${tag}"
