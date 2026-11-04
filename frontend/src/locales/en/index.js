// Bundles every namespace of this language into one object (one webpack chunk per language).
import common from "./common.json";
import nav from "./nav.json";
import auth from "./auth.json";
import pages from "./pages.json";
import dialogs from "./dialogs.json";
import toasts from "./toasts.json";
import errors from "./errors.json";

const resources = { common, nav, auth, pages, dialogs, toasts, errors };
export default resources;
