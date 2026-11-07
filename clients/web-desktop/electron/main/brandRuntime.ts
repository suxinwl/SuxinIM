import { app } from "electron";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import brand from "../../../brand.json";
import { runtimeUserData } from "./localStore/bootstrap";
import { legacyReaderRuntimeFromArgs } from './localStore/legacyReaderProcess';

// The published app's package name and data directory are upgrade identity,
// independent of configurable display names and installer filenames.
const userData = process.argv.includes('--puty-legacy-reader') ? legacyReaderRuntimeFromArgs() : runtimeUserData();
mkdirSync(userData, { recursive: true });
app.setPath("userData", userData);
app.setName(brand.displayName);
