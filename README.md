# PGDeploy

PGDeploy 是一个纯 Go 编写的 Windows 命令行工具，用于在 Windows 11 / Windows Server 2022+ 上安装并管理 PostgreSQL 18（EDB 官方 Windows x64 二进制 ZIP 包）。

它不依赖 shell、PowerShell、Python 或 MSI：解压、ACL、Windows 服务注册、`initdb` 与健康检查全部通过 Go 直接调用 Win32 API 与原生 PostgreSQL 程序完成。客户机器无需安装 Go 运行时，只需提供官方 ZIP。

## 特性

- **一键安装**：`pgdeploy.exe install`，密码自动生成、路径自动选择、配置固定安全默认值。
- **安全默认值**：仅监听 `127.0.0.1`，`scram-sha-256` 认证，虚拟服务账户 `NT SERVICE\<name>`，数据目录最小权限 DACL。
- **来源真实性**：内置 SHA-256 白名单 + 可选 `--sha256`；未经验证的归档**绝不执行**其中的可执行文件。
- **安全解压**：拒绝 zip-slip、绝对路径、ADS、保留设备名、zip 炸弹、重复/大小写冲突条目。
- **事务日志 + 状态机**：支持断点续装、幂等重跑、`repair` 自愈，日志为 JSON Lines。
- **结构化错误码**：每个失败映射到稳定的 `E_*` 错误码与退出码。

## 要求

- Windows 11 x64，或 Windows Server 2022/2025 x64
- 管理员权限（`install`/`start`/`stop`/`restart`/`repair`/`uninstall`/`credentials show` 需要）
- PostgreSQL 18 EDB 官方 ZIP：`postgresql-18.<小版本>-<构建号>-windows-x64-binaries.zip`

## 快速开始

当前开发版内置哈希白名单为空，安装时必须提供可信的 `--sha256`。

1. 将 `pgdeploy.exe` 与 PostgreSQL ZIP 放在**同一目录**。
2. 运行安装（若当前非管理员，程序会通过 UAC 自动提权）：

   ```
   pgdeploy.exe install --sha256 <从独立可信渠道取得的64位HEX>
   ```

3. 查看自动生成的数据库密码：

   ```
   pgdeploy.exe credentials show
   ```

## 命令

| 命令 | 作用 | 管理员权限 |
|---|---|---|
| `pgdeploy.exe install` | 扫描 ZIP、预检查、完整安装并启动 | 是，支持 UAC 提权 |
| `pgdeploy.exe verify` | 仅检查 ZIP、架构、目录、散列、兼容性，不写入 | 否 |
| `pgdeploy.exe status` | 展示安装状态、SCM 状态、端口、版本、数据目录 | 否 |
| `pgdeploy.exe start` | 启动服务并等待就绪 | 是 |
| `pgdeploy.exe stop` | 停止服务并等待完成 | 是 |
| `pgdeploy.exe restart` | 停止后启动并做连接检查 | 是 |
| `pgdeploy.exe repair` | 修复 ACL、服务配置、缺失的运行时文件 | 是 |
| `pgdeploy.exe doctor` | 逐项诊断，输出 PASS/WARN/FAIL/UNKNOWN | 否 |
| `pgdeploy.exe credentials show` | 在管理员授权下显示当前密码 | 是 |
| `pgdeploy.exe uninstall` | 停止/删除服务，删除整个实例目录及所有数据 | 是 |
| `pgdeploy.exe version` | 显示版本与构建元数据 | 否 |
| `pgdeploy.exe help` | 显示帮助 | 否 |

### `install` 参数

```
--zip <path>          指定 ZIP；缺省在 EXE 所在目录自动发现
--root <path>         实例安装根目录（绝对路径）
--service <name>      Windows 服务名，默认 PGDeployPostgreSQL18
--port <1..65535>     TCP 端口，默认 5432
--sha256 <64-hex>     从独立可信渠道取得的 ZIP SHA-256 预期值
--password-file <p>   指定密码文件（只读取，不删除）
--manual-start        手动启动；默认开机自启
--no-start            安装后不立即启动、不做在线 SQL 验证
--json                JSON 输出（绝不包含明文密码）
--verbose             详细诊断输出
```

`verify` 支持 `--zip`、`--sha256`、`--json`。`status/start/stop/restart/repair/doctor` 支持 `--root`、`--service`、`--json`。`repair` 额外支持 `--zip`、`--sha256`，用于从可信归档恢复运行时。`uninstall` 默认完整删除实例，需要输入服务名确认；脚本可使用 `--yes` 明确确认。旧参数 `--purge-data` 仍兼容，`--purge-data=false` 不再支持。

## 示例

自定义根目录、服务名与端口：

```
pgdeploy.exe install --root "D:\Customer Apps\PostgreSQL" --service CustomerPG18 --port 55432 --sha256 <来自独立可信渠道的64位HEX>
pgdeploy.exe status --root "D:\Customer Apps\PostgreSQL" --service CustomerPG18
pgdeploy.exe doctor --root "D:\Customer Apps\PostgreSQL" --service CustomerPG18
pgdeploy.exe repair --root "D:\Customer Apps\PostgreSQL" --service CustomerPG18
pgdeploy.exe uninstall --root "D:\Customer Apps\PostgreSQL" --service CustomerPG18
```

## 完整卸载

```text
pgdeploy.exe uninstall --root "D:\Customer Apps\PostgreSQL" --service CustomerPG18
# 非交互式卸载，明确确认永久删除：
pgdeploy.exe uninstall --root "D:\Customer Apps\PostgreSQL" --service CustomerPG18 --yes
```

卸载会停止并删除 Windows 服务，删除整个实例根目录，包括 `runtime`、`data`、`logs`、`secrets`、`state`、`staging` 及根目录内的其他文件。数据库无法通过本工具恢复，请先保存需要的备份。

默认布局的 `PGDeploy\instances`、`PGDeploy` 父目录为空时也会移除；自定义根目录的父目录及其他实例不会删除。实例目录外的原始 ZIP、安装工具、用户提供的密码文件和 Windows 系统事件记录不属于卸载清理范围。

文件被占用或服务仍处于待删除状态时，卸载返回失败并保留重试所需的状态，不会报告成功。无法确认目录归属时拒绝删除。

## 目录布局

```
<root>\
├── runtime\pgsql\   # 解压后的 PostgreSQL 运行时（服务账户只读/执行）
├── data\            # 数据目录（服务账户可写）
├── logs\
│   ├── installer.jsonl
│   └── postgres\
├── secrets\         # postgres-password.txt（服务账户不可读）
├── state\
│   ├── install.json
│   └── journal.jsonl
└── staging\         # 仅安装期间使用，完成后删除
```

## 构建

开发/构建机器需要 Go 1.27.x。客户机器无需 Go。

```powershell
# 静态发布构建（CGO_ENABLED=0, windows/amd64）
.\scripts\build.ps1
```

等价命令：

```
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o dist/pgdeploy.exe ./cmd/pgdeploy
```

## 测试

```powershell
go test ./...            # 单元测试
go vet ./...
go test -tags release ./internal/source/   # 发布门槛：正式版必须携带非空 SHA-256 白名单
```

端到端集成测试（需要真实 ZIP + 管理员权限 + `--sha256`）：

```powershell
$env:PGDEPLOY_INTEGRATION='1'
$env:PGDEPLOY_TEST_SHA256='<64-hex>'
$env:PGDEPLOY_TEST_ZIP='C:\Downloads\postgresql-18.<小版本>-<构建号>-windows-x64-binaries.zip'
go test -tags integration ./tests/
```

## Suggested Test Script

建议使用`postgresql-18.6-5-windows-x64-binaries`进行测试，其官方SHA256是`e2246ba91d22345bc3d017586c09ede52d9df180b1eeb480f050445f1cad84e2`

建议交付架构
```
MyCommercialSoftware/
├── app.exe
├── postgres-installer.exe
├── postgresql-18.6-5-windows-x64-binaries.zip
└── licenses/
    ├── PostgreSQL-LICENSE.txt
    └── THIRD-PARTY-NOTICES.txt
```

可用的脚本为：
```
mkdir "D:\Customer Apps\PostgreSQL"
.\pgdeploy.exe install --root "C:\Customer Apps\PostgreSQL" --service CustomerPG18 --port 5432 --sha256 e2246ba91d22345bc3d017586c09ede52d9df180b1eeb480f050445f1cad84e2 --password-file ".\PASSWORD"
```

## Issues for new machines

新机器可能需要安装Microsoft Visual C++ v14 Redistributable，考虑 https://github.com/abbodi1406/vcredist

签名验证过慢的问题：

关闭沙盒内的 Smart App Control

```
Set-ItemProperty `
  -Path "HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy" `
  -Name "VerifiedAndReputablePolicyState" `
  -Value 0

# 立即刷新代码完整性策略
CiTool.exe --refresh
```
## 关于来源真实性（重要）

`install` 只信任能验证来源的 ZIP：内置白名单命中，或操作者通过独立可信渠道提供 `--sha256`。两者都缺失时，`install` 会以 `E_SOURCE_UNVERIFIED` 失败且**不会执行归档内任何 EXE**。`verify` 在无可信哈希时仍可完成只读结构检查，但结果为 `SOURCE_AUTHENTICITY_UNVERIFIED`。

正式发布前，维护者必须获取官方 `postgresql-18.6-5-windows-x64-binaries.zip` 的真实 SHA-256 并写入 `internal/source/allowlist.go` 的白名单（发布门槛会阻止空白名单的正式版本）。切勿编造哈希。

## License

见 [LICENSE](LICENSE)。第三方依赖声明见 [NOTICE.md](NOTICE.md)。
