import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const storeFile = process.env.ATTENDANCE_DATA_DIR
  ? path.join(process.env.ATTENDANCE_DATA_DIR, "attendance_store.json")
  : path.join(__dirname, "attendance_store.json");

fs.mkdirSync(path.dirname(storeFile), { recursive: true });

if (!fs.existsSync(storeFile)) {
  fs.writeFileSync(storeFile, "{}");
}

export function loadStore() {
  try {
    return JSON.parse(fs.readFileSync(storeFile, "utf8"));
  } catch {
    return {};
  }
}

export function saveStore(store) {
  fs.writeFileSync(storeFile, JSON.stringify(store, null, 2));
}
