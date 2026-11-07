"""Create private local config without overwriting existing files (Python 3)."""
from pathlib import Path
import secrets
root=Path(__file__).resolve().parents[1]
for name in ('config','app','upload'):
    target=root/'manifest/config'/f'{name}.yaml'
    if target.exists():
        print(f'Preserved {target.relative_to(root)}')
        continue
    content=(target.parent/f'{name}.example.yaml').read_text(encoding='utf-8')
    while 'CHANGE_ME_WITH_INIT_LOCAL_CONFIG' in content:
        content=content.replace('CHANGE_ME_WITH_INIT_LOCAL_CONFIG',secrets.token_hex(16),1)
    target.write_text(content,encoding='utf-8')
    print(f'Created {target.relative_to(root)}')

hack=root/'hack/config.yaml'
if not hack.exists():
    hack.write_bytes((hack.parent/'config.example.yaml').read_bytes())
