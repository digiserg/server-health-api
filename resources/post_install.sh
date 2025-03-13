#!/bin/bash

useradd -s /bin/false --no-create-home -r health

[ -x /usr/bin/systemctl ] && systemctl daemon-reload