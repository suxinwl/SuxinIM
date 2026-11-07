import json
import pathlib
import plistlib
import tempfile
import unittest
from sync_brand import CLIENTS, rendered_files

class BrandTest(unittest.TestCase):
    def fixture(self, root, name='PUTY内部通讯', technical='PUTY_IM'):
        root.joinpath('brand.json').write_text(json.dumps({'displayName': name, 'technicalName': technical}), encoding='utf-8')
        for relative in ['web-desktop/index.html', 'web-desktop/electron-builder.json5', 'flutter/ios/Runner/Info.plist']:
            target = root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text((CLIENTS / relative).read_text(encoding='utf-8'), encoding='utf-8')

    def test_rename_updates_all_generated_names_without_changing_windows_identity(self):
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            self.fixture(root, '示例内部通讯', 'EXAMPLE_IM')
            outputs = rendered_files(root)
            builder = json.loads(outputs[root / 'web-desktop/electron-builder.json5'])
            self.assertEqual(builder['appId'], 'io.openim.enterprise.upgrade38')
            self.assertEqual(builder['productName'], '示例内部通讯')
            self.assertEqual(builder['executableName'], 'EXAMPLE_IM')
            self.assertEqual(builder['win']['artifactName'], 'EXAMPLE_IM-${version}-win-x64.${ext}')
            self.assertTrue(builder['nsis']['perMachine'])
            self.assertIn('<title>示例内部通讯</title>', outputs[root / 'web-desktop/index.html'])
            ios = plistlib.loads(outputs[root / 'flutter/ios/Runner/Info.plist'].encode())
            self.assertEqual(ios['CFBundleDisplayName'], '示例内部通讯')
            self.assertEqual(ios['CFBundleIdentifier'], '$(PRODUCT_BUNDLE_IDENTIFIER)')
            self.assertIn('示例内部通讯', outputs[root / 'flutter/openim_common/lib/src/brand.g.dart'])

    def test_generated_text_is_escaped_for_xml_and_dart(self):
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            self.fixture(root, 'Acme $研发 & <Tech>')
            outputs = rendered_files(root)
            self.assertIn(r'Acme \$研发', outputs[root / 'flutter/openim_common/lib/src/brand.g.dart'])
            ios = plistlib.loads(outputs[root / 'flutter/ios/Runner/Info.plist'].encode())
            self.assertEqual(ios['CFBundleDisplayName'], 'Acme $研发 & <Tech>')

    def test_invalid_names_and_filename_traversal_are_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            for name, technical in [(' ', 'PUTY_IM'), ('Name\nBreak', 'PUTY_IM'), ('Name', '../outside'), ('Name', 'bad name')]:
                self.fixture(root, name, technical)
                with self.assertRaises(ValueError):
                    rendered_files(root)

    def test_existing_windows_application_identity_cannot_be_replaced(self):
        with tempfile.TemporaryDirectory() as folder:
            root = pathlib.Path(folder)
            self.fixture(root)
            path = root / 'web-desktop/electron-builder.json5'
            builder = json.loads(path.read_text(encoding='utf-8'))
            builder['appId'] = 'foreign.app'
            path.write_text(json.dumps(builder), encoding='utf-8')
            with self.assertRaises(ValueError):
                rendered_files(root)

if __name__ == '__main__':
    unittest.main()
