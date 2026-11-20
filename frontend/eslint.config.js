// Focused lint: catches the mistakes a bundler happily ships (an undefined variable or
// JSX component throws at runtime, e.g. a removed import). Deliberately NOT a full
// style ruleset. craco's build-time eslint (craco.config.js) still runs hooks checks.
const react = require("eslint-plugin-react");
const hooks = require("eslint-plugin-react-hooks");
const globals = require("globals");

module.exports = [
  { ignores: ["build/**", "node_modules/**", "scripts/**", "craco.config.js", "tailwind.config.js", "postcss.config.js", "eslint.config.js"] },
  {
    files: ["src/**/*.{js,jsx}"],
    linterOptions: { reportUnusedDisableDirectives: "off" }, // exhaustive-deps directives are for craco's build-time eslint
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      parserOptions: { ecmaFeatures: { jsx: true } },
      globals: { ...globals.browser, process: "readonly" },
    },
    plugins: { react, "react-hooks": hooks },
    rules: {
      "no-undef": "error",
      "react/jsx-no-undef": "error",
      "react/jsx-uses-vars": "error", // mark JSX-used imports as used
      "react-hooks/rules-of-hooks": "error",
    },
  },
];
