#!/usr/bin/env bash

################################################################################
#                                                                              #
#  Copyright 2020 Broadcom. The term Broadcom refers to Broadcom Inc. and/or   #
#  its subsidiaries.                                                           #
#                                                                              #
#  Licensed under the Apache License, Version 2.0 (the "License");             #
#  you may not use this file except in compliance with the License.            #
#  You may obtain a copy of the License at                                     #
#                                                                              #
#     http://www.apache.org/licenses/LICENSE-2.0                               #
#                                                                              #
#  Unless required by applicable law or agreed to in writing, software         #
#  distributed under the License is distributed on an "AS IS" BASIS,           #
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.    #
#  See the License for the specific language governing permissions and         #
#  limitations under the License.                                              #
#                                                                              #
################################################################################

set -e

TOPDIR=$(git rev-parse --show-toplevel || echo ${PWD})
ARGV=()

for V in "$@"; do
    case "$V" in
    -v|--v|-v=*|--v=*) HAS_V=1 ;;
    -*logto*|--*logto*|-log_dir*|--log_dir*) HAS_LOG=1;;
    -server_crt|--server_crt|-server_crt=*|--server_crt=*) HAS_CERT=1 ;;
    -server_key|--server_key|-server_key=*|--server_key=*) HAS_CERT=1 ;;
    -client_auth|--client_auth|-client_auth=*|--client_auth=*) HAS_AUTH=1 ;;
    esac
    ARGV+=( $V )
done

source ${TOPDIR}/tools/test/env.sh

TELEMETRY_BIN=${TOPDIR}/build/bin/telemetry
if [[ ! -f ${TELEMETRY_BIN} ]]; then
    >&2 echo "error: Telemetry server not compiled"
    >&2 echo "Please run 'make telemetry' and try again"
    exit 1
fi

EXTRA_ARGS=()
[[ -z ${HAS_LOG}  ]] && EXTRA_ARGS+=( -logtostderr )
[[ -z ${HAS_V} ]]    && EXTRA_ARGS+=( -v 2 )
[[ -z ${HAS_CERT} ]] && EXTRA_ARGS+=( -insecure )
[[ -z ${HAS_AUTH} ]] && EXTRA_ARGS+=( -client_auth none )

EXTRA_ARGS+=( -allow_no_client_auth )

set -x
${TELEMETRY_BIN} "${EXTRA_ARGS[@]}" "${ARGV[@]}"
