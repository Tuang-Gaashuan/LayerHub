# Optional cloud email worker / 可选云端邮件工作器

The core app works without outbound email. This alpha worker currently validates **163 SMTP, port 465, implicit TLS**. Unverified/unbound recipients do not receive notifications. A successful SMTP submission is not proof that a person read the message.

核心功能不依赖邮件。成员自行绑定并验证邮箱；不绑定、不验证或关闭通知的成员不发信。配置发件账号不等于为所有成员自动绑定邮箱。

## Cloud components / 云端部署

Use a dedicated unprivileged account. The cloud only runs the worker; it reads the NAS outbox through `http://127.0.0.1:15181/internal/mail/` with a private bearer token. Nginx blocks this path publicly.

```sh
sudo useradd --system --user-group --home-dir /var/lib/layerhub-mail --shell /usr/sbin/nologin layerhub-mail
sudo install -d -m 750 -o root -g layerhub-mail /etc/layerhub-mail
sudo install -d -m 755 /usr/local/lib/layerhub-mail
sudo install -m 755 binaries/linux-amd64/layerhub-mail-worker /usr/local/lib/layerhub-mail/worker
sudo install -m 640 -o root -g layerhub-mail deploy/mail/smtp.json.example /etc/layerhub-mail/smtp.json
sudo nano /etc/layerhub-mail/smtp.json
```

For an extracted portable Linux release, replace `binaries/linux-amd64/layerhub-mail-worker` with `bin/layerhub-mail-worker`. / 解压后的 Linux 便携包使用 `bin/layerhub-mail-worker`，源码构建目录使用上面的路径。

Replace sender and SMTP authorization code, not the mailbox login password. Do not put credentials in command arguments or Git. Keep the file mode 640 with only the worker group allowed to read; the directory is 750.

在一个全新邮件安装中生成随机令牌，不覆盖已有文件：

```sh
sudo sh -c 'test ! -e /etc/layerhub-mail/relay.token && umask 077 && openssl rand -hex 32 > /etc/layerhub-mail/relay.token'
sudo chown root:layerhub-mail /etc/layerhub-mail/relay.token
sudo chmod 640 /etc/layerhub-mail/relay.token
sudo sh -c 'printf "%s\n" "LAYERHUB_PUBLIC_URL=https://print.example.com/" > /etc/layerhub-mail/worker.env'
sudo chmod 600 /etc/layerhub-mail/worker.env
```

Privately transfer the same token through your own pinned SSH connection and set `LAYERHUB_MAIL_TOKEN` in the NAS root-only environment file. Do not expose it via the webpage, logs or an issue. Set NAS `LAYERHUB_PUBLIC_URL` to the same public HTTPS origin. Restart the NAS only while idle.

确认全部占位值已替换、令牌一致后再启用；没有配置好时不要启动工作器：

```sh
sudo install -m 644 deploy/mail/layerhub-mail.service /etc/systemd/system/layerhub-mail.service
sudo systemctl daemon-reload
sudo systemctl enable --now layerhub-mail
sudo journalctl -u layerhub-mail -n 30 --no-pager
```

Members then use Personal account → Email notifications to request and verify a code, and choose start/finish/error preferences. Have one consenting tester verify inbox delivery first. This deployment guide itself sends no email and does not include an existing sender account or token.
