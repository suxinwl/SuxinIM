// Local-only MySQL bridge. Credentials stay in child environment, never output.
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { resolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const require = createRequire(join(root, "web/package.json"));
try {
  const config = require("yaml").parse(readFileSync(join(root, "manifest/config/config.yaml"), "utf8")).database.default;
  if (config.name !== "kaoqin_dev" || !["127.0.0.1", "localhost"].includes(config.host) || config.prefix !== "gf_") throw new Error("Unsupported local database");
  const mode = process.argv[2];
  if (!["query", "backup"].includes(mode)) throw new Error("Unknown operation");
  const mysql = process.env.MYSQL_CLI || "C:/Program Files/MySQL/MySQL Server 8.0/bin/mysql.exe";
  const executable = mode === "backup" ? (process.env.MYSQLDUMP_CLI || join(dirname(mysql), "mysqldump.exe")) : mysql;
  const connection = ["--protocol=TCP", "--host=" + config.host, "--port=" + config.port, "--user=" + config.user, "--default-character-set=utf8mb4"];
  const args = mode === "backup" ? [...connection, "--single-transaction", "--quick", "--hex-blob", "--set-gtid-purged=OFF", "--skip-dump-date", "--order-by-primary", config.name, "biz_att_rule", "biz_att_audit"] : [...connection, "--batch", "--skip-column-names", "--raw", "--database=" + config.name];
  const result = spawnSync(executable, args, { input: mode === "query" ? readFileSync(0) : undefined, windowsHide: true, env: { ...process.env, MYSQL_PWD: String(config.pass) }, maxBuffer: 64 * 1024 * 1024, timeout: 120000 });
  if (result.error || result.status) {
    const match = String(result.stderr || "").match(/ERROR\s+(\d+)\s+\([^)]*\)\s+at line\s+(\d+)/);
    process.stderr.write(match ? `MYSQL_ERROR ${match[1]} LINE ${match[2]}\n` : "Local database operation failed; private diagnostics suppressed\n");
    process.exit(1);
  }
  process.stdout.write(result.stdout);
} catch {
  process.stderr.write("Local database bridge failed; configuration and credentials suppressed\n");
  process.exit(1);
}
