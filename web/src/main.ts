// Entry point of the generated site's single bundle.js: each page only
// activates the part whose DOM it actually contains.

import { initChangelog } from "./changelog";
import { initOutdated } from "./outdated";

initChangelog();
initOutdated();
