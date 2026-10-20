#!/bin/sh
# flaky.sh CMD...: run CMD, except on server1, which has "bad memory":
# there it dies of SIGBUS, so that balance can retry it on server2.
#
# Demo only: your project does not need this file.
case $PWD in */server1/*) kill -BUS $$ ;; esac
exec "$@"
