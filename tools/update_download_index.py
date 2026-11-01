"""Render internal downloads from the verified, explicitly published release report."""
import html,json,pathlib,urllib.parse
ROOT=pathlib.Path(__file__).resolve().parents[1]
report=json.loads((ROOT/'tests/release-downloads.json').read_text(encoding='utf-8'))
if not report.get('published') or not report.get('fullHTTPSDownloadsVerified'):raise SystemExit('Publish and verify the full packages first')
retired={pathlib.PurePosixPath(urllib.parse.urlsplit(item.get('url','')).path).name for path in (ROOT/'tests').glob('*withdrawal.json') for item in json.loads(path.read_text(encoding='utf-8')).get('withdrawn',[])}
entries={item['platform']:item for item in report['artifacts']};rows=[]
for platform,label in [('windows','Windows x64'),('android','Android ARM64')]:
 item=entries.get(platform)
 if not item:
  rows.append('<li>'+label+'：运行验收中，暂未发布。</li>');continue
 name=pathlib.PurePosixPath(urllib.parse.urlsplit(item['url']).path).name
 if name in retired:raise SystemExit('Refusing withdrawn package: '+name)
 if platform=='windows':note='测试包未作 Windows 代码签名。安装前导入本项目开发 CA。'
 else:note='项目专用签名测试包。固定 Go SDK 需要系统信任或公信服务器证书；仅安装普通用户 CA 不能保证连接。尚未验证物理手机。'
 rows.append('<li><a href="'+html.escape(urllib.parse.quote(name),quote=True)+'">'+label+' '+html.escape(item['version'])+'（'+str(item['build'])+'）</a><br><small>'+note+'<br>'+str(item['size'])+' 字节<br>SHA-256：<code>'+html.escape(item['sha256'])+'</code></small></li>')
page='''<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>OpenIM38 隔离测试下载</title><style>body{font:16px/1.7 system-ui,sans-serif;max-width:820px;margin:40px auto;padding:0 20px;color:#172033;background:#f6f8fc}main{padding:30px;border-radius:14px;background:white}a{color:#125cc8}li{margin:18px 0}code{overflow-wrap:anywhere}small{color:#526077}</style><main><h1>OpenIM38 内网测试下载</h1><p>原企业项目合并开发版，使用合成账号和数据，完整升级验收尚未全部结束。</p><ul>'''+''.join(rows)+'''<li><a href="OpenIM38-Development-CA.zip">Windows 开发证书与安装检测脚本</a><br><small>解压后以运行客户端的同一用户执行 Install-and-check.cmd，核对证书名称 OpenIM38 Isolated Development CA。压缩包仅含公钥证书及脚本。</small></li></ul><p><a href="/">打开 Web 通讯</a> · <a href="/admin/">打开管理后台</a></p><p>此处仅用于独立测试端口 29880/29885。本期不接 HRM；iOS 提供源码与 Mac/Xcode 构建说明。</p></main></html>'''
destination=ROOT/'artifacts/downloads/index.html';destination.parent.mkdir(parents=True,exist_ok=True);destination.write_text(page+'\n',encoding='utf-8')
print(json.dumps({'platforms':list(entries),'index':destination.relative_to(ROOT).as_posix()}))
