"""Static UI/HTTP inventory from preserved enterprise source and current clients.

This is a source mapping, never a page-by-page runtime acceptance report.
No files in original sources or clients are modified.
"""
from __future__ import annotations

import datetime
import hashlib
import json
import os
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[1]
BASELINE = ROOT / "docs/provenance/original-source-sha256.json"
SOURCE_EXTENSIONS = {".ts", ".tsx", ".js", ".jsx", ".dart"}
SKIP = {"node_modules", ".git", ".dart_tool", ".gradle", "build", "dist", "dist-web", ".umi", ".umi-production", "migration-reference"}
LITERAL = re.compile(r"(['\"`])((?:\\.|(?!\1)[^\r\n])*?)\1")
ROUTE_PATH = re.compile(r"\bpath\s*:\s*(['\"])([^'\"\r\n]+)\1")
FLUTTER_ROUTE = re.compile(r"\bname\s*:\s*((?:\w+Routes|AppRoutes|MRoutes)\.\w+|['\"][^'\"\r\n]+['\"])")


def sha(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def masks(text):
    """Preserve line/character positions; separately blank comments and strings."""
    visible, structure = list(text), list(text)
    i = 0
    while i < len(text):
        if text.startswith("//", i) or text.startswith("/*", i):
            end = text.find("\n", i) if text.startswith("//", i) else text.find("*/", i + 2)
            end = len(text) if end < 0 else end if text.startswith("//", i) else end + 2
            for j in range(i, end):
                if text[j] != "\n":
                    visible[j] = structure[j] = " "
            i = end
        elif text[i] in "'\"`":
            quote, j = text[i], i + 1
            while j < len(text):
                if text[j] == "\\":
                    j += 2
                elif text[j] == quote:
                    j += 1
                    break
                else:
                    j += 1
            for k in range(i, min(j, len(text))):
                if text[k] != "\n":
                    structure[k] = " "
            i = j
        else:
            i += 1
    return "".join(visible), "".join(structure)


def source_files(root):
    if not root.is_dir():
        return []
    result = []
    for directory, directories, files in os.walk(root):
        directories[:] = [name for name in directories if name not in SKIP]
        for name in files:
            path = pathlib.Path(directory) / name
            if path.suffix in SOURCE_EXTENSIONS:
                result.append(path)
    return sorted(result)


def line(text, offset):
    return text.count("\n", 0, offset) + 1


def is_page(relative):
    parts = relative.parts
    return "pages" in parts or relative.name.endswith(("_view.dart", "_page.dart", "_screen.dart"))


def is_routes(relative):
    return "routes" in relative.parts or relative.name in {"routes.ts", "routes.dart"} or relative.name.endswith(("_routes.dart", "_pages.dart"))


def is_api(relative):
    value = relative.as_posix().lower()
    return ("/api/" in "/" + value or "/services/" in "/" + value
            or relative.stem.lower() in {"api", "apis", "urls", "http_util", "enterprise", "enterprise_manager", "imfacade"}
            or relative.stem.lower().endswith("_api") or value.startswith("src/config/"))


def region(structure, offset, opener="{", closer="}"):
    stack, intervals = [], []
    for i, char in enumerate(structure):
        if char == opener:
            stack.append(i)
        elif char == closer and stack:
            start = stack.pop()
            if start <= offset <= i:
                intervals.append((start, i + 1))
    return max(intervals, default=(max(0, offset - 250), min(len(structure), offset + 600)), key=lambda pair: pair[0])


def route_entries(text, relative, constants):
    visible, structure = masks(text)
    pattern = FLUTTER_ROUTE if relative.suffix == ".dart" else ROUTE_PATH
    entries = []
    for match in pattern.finditer(text):
        commented = visible[match.start():match.end()].strip() == ""
        reference = match.group(1) if relative.suffix == ".dart" else match.group(2)
        value = constants.get(reference, reference.strip("'\""))
        start, end = region(structure, match.start(), "(", ")") if relative.suffix == ".dart" else region(structure, match.start())
        block = text[start:end] if not commented else text[max(0, match.start() - 100):match.end() + 450]
        component = re.search(r"\b(?:component|element|page)\s*:\s*([^\r\n,]+)", block)
        visibility = "commented_out_declaration" if commented else "active_declaration_reachability_not_proven"
        if not commented and re.search(r"\bhideInMenu\s*:\s*true", block):
            visibility = "hideInMenu_literal_in_enclosing_block; scope_requires_review"
        entries.append({"pathOrReference": value, "reference": reference, "line": line(text, match.start()),
                        "declarationState": visibility, "componentExpressionCandidate": component.group(1).strip() if component else None,
                        "runtimeVerification": "not_asserted_by_static_inventory"})
    return entries


def constant_routes(files, root):
    constants = {}
    for path in files:
        if path.suffix != ".dart" or not is_routes(path.relative_to(root)):
            continue
        visible, _ = masks(path.read_text(encoding="utf-8", errors="replace"))
        class_match = re.search(r"\bclass\s+(\w+Routes|AppRoutes|MRoutes)\b", visible)
        if class_match:
            for name, value in re.findall(r"\bstatic\s+const\s+(\w+)\s*=\s*['\"]([^'\"]+)['\"]", visible):
                constants[class_match.group(1) + "." + name] = value
    return constants


def nearest_symbol(text, offset):
    declarations = list(re.finditer(
        r"(?:export\s+)?(?:async\s+)?function\s+(\w+)|(?:export\s+)?(?:const|let)\s+(\w+)\s*=|"
        r"\bstatic\s+(?:final|const)\s+(\w+)\s*=|(?:static\s+)?(?:Future(?:<[^\n]+?>)?|void|bool|String)\s+(\w+)\s*\(", text[:offset]))
    if not declarations:
        return None
    return next((value for value in declarations[-1].groups() if value), None)


def http_entries(text, relative):
    visible, _ = masks(text)
    entries = []
    for match in LITERAL.finditer(text):
        value = match.group(2)
        # Only API/config modules are eligible. Keep interpolation explicitly;
        # it is not proof of the resulting hostname, method or runtime request.
        if not (value.startswith("/") and len(value) > 1 or re.match(r"^(?:\$\{[^}]+\}|\$\w+|https?://)[^\s]*\/", value)):
            continue
        if re.search(r"\.(?:png|jpg|svg|json|css|tsx?|dart)(?:$|\?)", value, re.I):
            continue
        prefix_removed = re.sub(r"^\$\{[^}]+\}|^\$\w+|^https?://[^/]+", "", value)
        commented = visible[match.start():match.end()].strip() == ""
        before, after = text[max(0, match.start() - 220):match.start()], text[match.end():match.end() + 220]
        method_match = re.search(r"\.(get|post|put|patch|delete)\s*\(\s*$", before, re.I)
        option_method = re.search(r"\bmethod\s*:\s*['\"](GET|POST|PUT|PATCH|DELETE)['\"]", after, re.I)
        method = method_match.group(1).upper() if method_match else option_method.group(1).upper() if option_method else None
        category = "template_or_demo_module" if any(part in {"swagger", "ant-design-pro"} for part in relative.parts) else "api_literal_candidate"
        entries.append({"literal": value, "pathAfterLeadingBasePlaceholderRemoved": prefix_removed,
                        "line": line(text, match.start()), "symbol": nearest_symbol(visible, match.start()),
                        "httpMethodLiteral": method, "state": "commented_out_literal" if commented else "active_source_literal",
                        "category": category, "runtimeVerification": "not_asserted_by_static_inventory"})
    return entries


def adapter_refs(text):
    return [{"line": index, "symbol": match.group(0)}
            for index, value in enumerate(text.splitlines(), 1)
            for match in re.finditer(r"\b(?:ImFacade|AppIm|enterpriseRequest|EnterpriseManager|PublicMessageAdapter|createImFacade|IMSDK|OpenIMSDK)\b", value)]


def component_inventory(original, current, baseline_files):
    old_files, new_files = source_files(original), source_files(current)
    old_relative_files = {path.relative_to(original).as_posix() for path in old_files}
    new_text = {path.relative_to(current).as_posix(): path.read_text(encoding="utf-8", errors="replace") for path in new_files}
    old_constants, new_constants = constant_routes(old_files, original), constant_routes(new_files, current)
    new_routes, new_http = [], []
    for relative, text in new_text.items():
        path = pathlib.PurePosixPath(relative)
        if is_routes(path):
            new_routes.extend({"file": relative, **entry} for entry in route_entries(text, path, new_constants))
        if is_api(path):
            new_http.extend({"file": relative, **entry} for entry in http_entries(text, path))
    pages, routes, endpoints, mismatches = [], [], [], []
    new_by_name = {}
    for name in new_text:
        new_by_name.setdefault(pathlib.PurePosixPath(name).name, []).append(name)
    checked = set()
    for path in old_files:
        relative = path.relative_to(original)
        if not (is_page(relative) or is_routes(relative) or is_api(relative)):
            continue
        name, text = relative.as_posix(), path.read_text(encoding="utf-8", errors="replace")
        digest = sha(path)
        checked.add(name)
        if name not in baseline_files or digest != baseline_files[name]["sha256"]:
            mismatches.append(name)
        target = current / relative
        exact = name in new_text
        candidates = [name] if exact else new_by_name.get(path.name, [])
        state = "retained_same_relative_path_unchanged" if exact and sha(target) == digest else "retained_same_relative_path_modified" if exact else "relocation_candidate_not_proven" if candidates else "no_static_file_match_review_feature_matrix"
        if is_page(relative):
            pages.append({"originalFile": name, "originalSHA256": digest, "baselineSHA256": baseline_files.get(name, {}).get("sha256"),
                          "mappingState": state, "newFileCandidates": [str((current / item).relative_to(ROOT)).replace("\\", "/") for item in candidates],
                          "newAdapterReferences": adapter_refs(new_text[name]) if exact else [],
                          "originalVisibility": "page_source_exists; menu/route reachability not inferred",
                          "runtimeVerification": "not_asserted_by_static_inventory"})
        if is_routes(relative):
            for entry in route_entries(text, relative, old_constants):
                matches = [x for x in new_routes if x["pathOrReference"] == entry["pathOrReference"]]
                routes.append({"originalFile": name, **entry, "newDeclarationCandidates": matches,
                               "mappingState": "same_relative_file_and_route_literal" if any(x["file"] == name for x in matches) else "matching_route_literal_other_file_review" if matches else "no_static_route_match_review_feature_matrix"})
        if is_api(relative):
            for entry in http_entries(text, relative):
                matches = [x for x in new_http if x["pathAfterLeadingBasePlaceholderRemoved"] == entry["pathAfterLeadingBasePlaceholderRemoved"]
                           or x["file"] == name and entry["symbol"] and x["symbol"] == entry["symbol"]]
                endpoints.append({"originalFile": name, **entry, "newLiteralCandidates": matches,
                                  "newSameFileAdapterReferences": adapter_refs(new_text[name]) if exact else [],
                                  "mappingState": "static_literal_or_same_symbol_candidate" if matches else "no_literal_match_may_use_sdk_or_adapter_review_feature_matrix"})
    return {"originalSourcePresent": original.is_dir(), "currentRoot": current.relative_to(ROOT).as_posix(),
            "baselineRelevantFilesChecked": len(checked), "baselineRelevantFileMismatches": mismatches,
            "pageSourceFiles": pages, "routeDeclarations": routes, "httpLiteralDeclarations": endpoints,
            "currentRouteDeclarations": new_routes, "currentHttpLiteralDeclarations": new_http,
            "newPageSourceFilesWithoutSameOriginalRelativePath": [name for name in new_text if is_page(pathlib.PurePosixPath(name)) and name not in old_relative_files]}


def main():
    baseline = json.loads(BASELINE.read_text(encoding="utf-8"))
    roots = {"openim-electron-enterprise-toc-enterprise-rtc": ("webWindows", "clients/web-desktop"),
             "open-im-flutter-enterprise-toc-rtc-3.5.1-e-1.1.4": ("flutter", "clients/flutter"),
             "openim-admin-3": ("admin", "clients/admin")}
    source_records, components = [], {}
    for item in baseline:
        source = pathlib.Path(item["source"])
        kind = roots.get(source.name)
        current = kind[1] if kind else "services/openim-server" if source.name.startswith("open-im-server") else "services/chat"
        source_records.append({"originalSource": item["source"], "baselineFileCount": item["fileCount"],
                               "sourcePresent": source.is_dir(), "currentRoot": current,
                               "inventoryRole": "client_ui_and_http" if kind else "server_baseline_identity_only; RPC/API migration documented separately"})
        if kind:
            components[kind[0]] = component_inventory(source, ROOT / current, {x["path"]: x for x in item["files"]})
    layers = ["clients/web-desktop/src/im/ImFacade.ts", "clients/web-desktop/src/im/enterprise.ts",
              "clients/flutter/local_plugin/app_im/lib/src/enterprise_manager.dart", "clients/flutter/openim_common/lib/src/utils/http_util.dart",
              "clients/admin/src/config/index.ts", "clients/admin/src/app.tsx",
              "services/enterprise-service/internal/enterprise/admin_proxy.go", "services/enterprise-service/internal/enterprise/server.go"]
    report = {"generatedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(), "tool": "tools/feature_inventory.py",
              "baselineManifest": BASELINE.relative_to(ROOT).as_posix(), "baselineManifestSHA256": sha(BASELINE),
              "purpose": "逐路由/页面源文件/HTTP字面量的静态保留及适配候选清单；不是逐项运行验收",
              "baselineProjects": source_records, "components": components,
              "adapterLayerEvidence": [{"file": p, "sha256": sha(ROOT / p)} for p in layers if (ROOT / p).is_file()],
              "adapterDispatchNotes": {
                  "webWindows": "Current page references to IMSDK are serviced by src/im/ImFacade.ts; enterpriseRequest builds getEnterpriseUrl()+path. An import/reference alone does not prove a particular SDK method executed.",
                  "flutter": "AppIm wraps public SDK methods and EnterpriseManager; openim_common HttpUtil rewrites office/rtc URL paths to enterprise transport. Interpolated/chained URL constants remain literal candidates, not resolved endpoints.",
                  "admin": "src/config/index.ts defines IM /enterprise/v1/admin/im/, Chat admin /admin/chat/, business /admin/business/. BFF admin_proxy.go validates and dispatches upstream requests. Relative API literals must be combined with the call's baseURL; this inventory does not infer absent baseURL data flow."},
              "limits": ["Regex/lexical source extraction, not a TypeScript/Dart compiler or runtime crawler.",
                         "Route component expressions and relocation candidates require review; nested blocks, conditional menus, permission guards and composed relative routes are not fully evaluated.",
                         "Commented-out route/literal declarations are listed separately; source retention is not proof the original menu was visible or a page can be reached.",
                         "HTTP inventory covers literal candidates in API/config modules; dynamically assembled URLs, SDK native/WASM RPCs and calls through variable references are not an exhaustive protocol enumeration.",
                         "Template/demo API modules are labeled and must not be mistaken for active product features.",
                         "No entry is marked runtime passed. Use docs/feature-matrix.md and docs/acceptance.md for actual evidence and unresolved cases.",
                         "Original server projects are included as baseline identities; this UI-oriented tool does not claim to exhaustively map their internal Go RPC routes."],
              "originalSourceModified": False, "clientSourceModified": False}
    output = ROOT / "docs/provenance/ui-api-inventory.json"
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"output": output.relative_to(ROOT).as_posix(), "components": {name: {"pages": len(value["pageSourceFiles"]), "routes": len(value["routeDeclarations"]), "httpLiterals": len(value["httpLiteralDeclarations"]), "baselineMismatches": len(value["baselineRelevantFileMismatches"])} for name, value in components.items()}}, ensure_ascii=False))


if __name__ == "__main__":
    main()
