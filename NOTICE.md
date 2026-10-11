# NOTICE

PGDeploy 使用并分发以下第三方组件。

## golang.org/x/sys

- 项目地址：https://cs.opensource.google/go/x/sys
- 许可：BSD 3-Clause "New" or "Revised" License

该模块提供 Windows 系统调用绑定（`windows`、`windows/svc`、`windows/svc/mgr`），用于 SCM 服务管理、令牌提升检测、ACL/SDDL 处理与已知目录解析。

```
Copyright (c) 2009 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

   * Redistributions of source code must retain the above copyright
     notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
     copyright notice, this list of conditions and the following
     disclaimer in the documentation and/or other materials provided
     with the distribution.
   * Neither the name of Google Inc. nor the names of its
     contributors may be used to endorse or promote products derived
     from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## PostgreSQL / EDB

PGDeploy 本身不重新分发 PostgreSQL 二进制文件；它由操作者提供官方
`postgresql-18.x-y-windows-x64-binaries.zip` 归档。PostgreSQL 是 PostgreSQL
全球开发组的注册商标，其授权见 PostgreSQL License（https://www.postgresql.org/about/licence/）。
EDB 提供的 Windows 二进制包遵循其各自发布条款。
