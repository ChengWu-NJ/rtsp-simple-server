#!/usr/bin/bash
set -e

SVCNAME=tpcmediasvr
SVCUSER=ubuntu    ### could be changed to another user with service permissions
CONFFILE="rtsp-simple-server.yml"
WORKDIR="/opt/"${SVCNAME}

cd ./bin/${SVCNAME}
go build

sudo systemctl stop ${SVCNAME} &>/dev/null || echo -n
sudo systemctl disable ${SVCNAME} &>/dev/null || echo -n

sudo mkdir -p ${WORKDIR}
sudo cp ./${SVCNAME} ${WORKDIR}
if [[ ! -f ${WORKDIR}/${CONFFILE} ]];then
  sudo cp ${CONFFILE} ${WORKDIR}
fi

cat > /tmp/${SVCNAME}.service <<EOF
[Unit]
Description=TPC Media Server
After=network-online.target syslog.target

[Service]
Type=simple
WorkingDirectory=${WORKDIR}
User=${SVCUSER}
ExecStart=${WORKDIR}/${SVCNAME}
Restart=on-failure
LimitNOFILE=65536
Restart=on-abnormal
RestartSec=65s
TimeoutSec=0
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=${SVCNAME}

[Install]
WantedBy=multi-user.target
EOF

sudo cp /tmp/${SVCNAME}.service /etc/systemd/system/

sudo systemctl start ${SVCNAME} &>/dev/null || echo -n
sudo systemctl enable ${SVCNAME} &>/dev/null || echo -n

sudo systemctl status ${SVCNAME}
echo -e "You could use the following command to monitor the server:\njournalctl -n 100 -f -u ${SVCNAME}"
