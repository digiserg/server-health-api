#!/bin/bash

getent passwd health > /dev/null || useradd -s /bin/false --no-create-home -r health

[ -x /usr/bin/systemctl ] && systemctl daemon-reload