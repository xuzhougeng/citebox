# Windows Authenticode 签名

正式 tag（`v*`）触发的 GitHub Actions 会用 SSL.com eSigner CodeSignTool 给 Windows 二进制做 Authenticode 签名，并加上 RFC 3161 时间戳。时间戳地址是 CodeSignTool 自带配置里的 `TSA_URL=http://ts.ssl.com`。

签名发生在打包之前：

- 服务端：签名包目录根下的 `citebox.exe`（以及同级 `.dll` / `.msi` / `.sys`），再把目录打成 `citebox-windows-{version}.zip`。
- 桌面端：签名 payload 根下的 `citebox-desktop.exe`（以及同级相关二进制），再用 NSIS 生成安装包，然后签名 `citebox-desktop-windows-{version}.exe`。

zip 不会被签名。安装包是用户直接运行的 `.exe`，所以在 NSIS 完成后再签一次。NSIS 在用户机器上安装时才生成 `Uninstall.exe`，它不是发布包里的预置文件。

## 密钥

把下面的值配置在仓库的 Settings → Secrets and variables → Actions。不要把值写进仓库、日志或工作流文件。

| Secret | 含义 |
| --- | --- |
| `ESIGNER_USERNAME` | SSL.com 账号用户名 |
| `ESIGNER_PASSWORD` | SSL.com 账号密码，传给 CodeSignTool 的 `-password` |
| `ESIGNER_CREDENTIAL_ID` | eSigner 代码签名证书的 Credential ID |
| `ESIGNER_TOTP_SECRET` | eSigner 自动化签名用的 TOTP 密钥，不是一次性验证码 |
| `ESIGNER_PIN` | 证书登记 eSigner 时设置的 4 位 PIN |

当前固定使用的 CodeSignTool `v1.3.2` 的 `sign` 命令没有 `-pin` 参数。tag 发布仍然要求 `ESIGNER_PIN` 已设置，这样 CiteBox 和其他仓库使用同一组 eSigner 密钥。如果以后换上的 CodeSignTool 在 `sign --help` 里提供 `-pin`，签名脚本会把 PIN 传进去。

`v1.3.2` 的 `main` 会丢掉 picocli 的退出码。认证失败、`invalid otp`、参数错误时进程退出码仍是 0，原因写在标准输出的 `Error: ...`、标准错误的异常栈，或 `logs/code_signing_tool.log`。签名脚本用 .NET `Process` 直接启动 `java`，把用户名、密码、Credential ID、TOTP 和可选的 PIN 作为单独参数传入。它把下面任一情况当成签名失败，并把脱敏后的工具输出打到日志：

- 进程退出码不是 0，或调用超时
- 标准输出或标准错误里出现 `Error:` 或异常栈
- 没有 `Code signed successfully:`
- 输出目录里没有签好的文件

`invalid otp` 或 `OTP is invalid` 会等到下一个 TOTP 窗口再试，最多 4 次。Windows 桌面 job 比服务端 job 多等一个窗口，避免两个并行 job 用同一组验证码重试。日志和失败信息里的密钥值会替换成 `***`。

tag 发布（`github.event_name == push`）会在构建前检查这五个密钥。缺任何一个，Windows job 失败，Release 不会挂上未签名的 Windows 产物。签名或 Authenticode 校验失败同样会使 job 失败，工作流没有 `continue-on-error`。

`workflow_dispatch` 不会创建 GitHub Release。五个密钥都没配置时跳过签名；只配置了其中一部分，或密钥齐全但签名失败时，job 失败。本地 `make package-windows` 和 `make package-desktop-windows` 使用同一规则：五个变量都未设置时跳过签名，变量不全或签名失败时停止打包。

Windows runner 会安装 Temurin 17，用来运行 CodeSignTool。

## SmartScreen

Authenticode 签名用来标明发布者，RFC 3161 时间戳让签名在证书过期后仍然可以校验。Microsoft SmartScreen 的信誉是另一套数据：新证书一开始几乎没有下载信誉，用户仍可能看到“Windows 已保护你的电脑”。已签名程序被实际下载得越多，这个信誉才会逐渐积累。第一次发出签名包不会立刻消除 SmartScreen 提示。

## 校验

在 Windows 上对签名后的 exe 运行：

```powershell
Get-AuthenticodeSignature .\citebox.exe | Format-List Status, StatusMessage, SignerCertificate, TimeStamperCertificate
signtool verify /pa /v .\citebox.exe
```

发布 job 在替换原文件之前做同样的检查：`Status` 必须是 `Valid`，并且要有时间戳证书。runner 上如果能找到 Windows SDK 的 `signtool.exe`，还会执行 `signtool verify /pa /v`，退出码必须为 0。

对桌面安装包，对 `citebox-desktop-windows-{version}.exe` 做同样的检查。zip 里应该是已签名的 `citebox.exe`，zip 文件本身没有 Authenticode 签名。
