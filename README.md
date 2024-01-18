# Usage

```sh
go run main.go <target_without_path>
```
or use the precompiled binary
```bat
logger.exe <target_without_path>
```
and then in tavern proxy
```
http://localhost:9090/proxy/openai
```
for example.
Logs appear in stdout as well as in `logger.log`
