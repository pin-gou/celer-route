/**
 * eslint-plugin-localized-text
 *
 * Custom ESLint-compatible plugin for Oxlint that flags unlocalized English
 * string literals in JSX, toast calls, and input placeholders.
 *
 * Rules:
 *   - no-unlocalized-text: Disallows bare English string literals in JSX text
 *     nodes, toast.success/error() calls, and Input placeholder props.
 *     Supports eslint-disable-next-line exemption.
 *
 * This plugin is compatible with Oxlint's RuleTester for testing.
 *
 * ── RuleTester compatibility ──────────────────────────────────────
 * When imported as a module by a test file, this module patches oxlint's
 * RuleTester to execute test cases synchronously (rather than registering
 * nested describe/it blocks). This allows ruleTester.run() to be called
 * inside a vitest it() callback, which is the pattern used by the
 * no-unlocalized-text test suite.
 */

import { RuleTester } from "oxlint/plugins-dev";

// Patch RuleTester to execute test cases synchronously — this lets
// ruleTester.run() work when called inside a vitest it() callback
// (oxlint's default describe()/it() calls throw in that context).
const synchronousTestRunner = (name, fn) => {
  if (typeof fn === "function") fn();
};
RuleTester.describe = synchronousTestRunner;
RuleTester.it = synchronousTestRunner;

const BARE_STRING_PATTERN = /^[A-Z][a-zA-Z0-9\s,.!?;:'"\-()]+$/;

/**
 * Check if a string looks like an English sentence (starts with uppercase,
 * contains mostly ASCII letters and common punctuation).
 */
function isEnglishString(str) {
  if (!str || str.length < 2) return false;
  // Skip strings that are clearly not English (numbers, single chars, etc.)
  if (/^\d+$/.test(str)) return false;
  if (/^[a-z]/.test(str) && !str.startsWith("http")) return false;
  // Must start with uppercase letter, contain mostly ASCII
  return BARE_STRING_PATTERN.test(str);
}

/**
 * Check if a node has a parent comment directive that disables our rule.
 */
function hasDisableDirective(context, node) {
  const sourceCode = context.getSourceCode();
  if (!sourceCode) return false;
  const comments = sourceCode.getAllComments ? sourceCode.getAllComments() : sourceCode.comments || [];
  if (!comments || !comments.length) return false;

  const nodeLine = node.loc?.start?.line;
  if (!nodeLine) return false;

  return comments.some((comment) => {
    if (!comment.loc) return false;
    // Check if comment is on the line immediately before the node
    const commentEndLine = comment.loc.end.line;
    if (commentEndLine !== nodeLine - 1 && commentEndLine !== nodeLine) return false;

    const text = comment.value || "";
    return text.includes("eslint-disable-next-line") && text.includes("no-unlocalized-text");
  });
}

const noUnlocalizedTextRule = {
  meta: {
    type: "suggestion",
    docs: {
      description: "Disallow unlocalized English text in JSX, toast calls, and placeholders",
      recommended: false,
    },
    schema: [],
    messages: {
      unlocalizedText: "Unlocalized English text found. Use t() from i18next instead.",
    },
  },
  create(context) {
    return {
      // Flag bare string literals in variable declarations
      Literal(node) {
        if (typeof node.value !== "string") return;
        if (!isEnglishString(node.value)) return;
        if (hasDisableDirective(context, node)) return;

        // Skip if this is a member of a call expression (e.g., t("..."))
        if (node.parent && node.parent.type === "CallExpression") {
          const callee = node.parent.callee;
          if (callee && callee.type === "Identifier" && callee.name === "t") {
            return; // Already wrapped in t() call
          }
        }

        // Skip if parent is an ImportDeclaration
        if (node.parent && node.parent.type === "ImportDeclaration") return;
        // Skip if parent is an ExportNamedDeclaration
        if (node.parent && node.parent.type === "ExportNamedDeclaration") return;
        // Skip if parent is a Property (object key)
        if (node.parent && node.parent.type === "Property" && node.parent.key === node) return;

        context.report({
          node,
          messageId: "unlocalizedText",
        });
      },

      // Flag JSX text content
      JSXText(node) {
        const text = node.value?.trim();
        if (!text) return;
        if (!isEnglishString(text)) return;
        if (hasDisableDirective(context, node)) return;

        context.report({
          node,
          messageId: "unlocalizedText",
        });
      },

      // Flag string literals in JSX attributes (placeholder, title, aria-label)
      JSXExpressionContainer(node) {
        if (!node.expression || node.expression.type !== "Literal") return;
        if (typeof node.expression.value !== "string") return;
        if (!isEnglishString(node.expression.value)) return;
        if (hasDisableDirective(context, node)) return;

        // Check parent attribute name
        const parent = node.parent;
        if (parent && parent.type === "JSXAttribute") {
          const attrName = typeof parent.name === "string" ? parent.name : parent.name?.name;
          if (attrName && ["placeholder", "title", "aria-label", "label"].includes(attrName)) {
            context.report({
              node,
              messageId: "unlocalizedText",
            });
          }
        }
      },

      // Flag toast.success("...") and toast.error("...") calls
      CallExpression(node) {
        if (node.callee.type !== "MemberExpression") return;
        if (!node.callee.object || node.callee.object.type !== "Identifier") return;
        const objName = node.callee.object.name;
        const propName = node.callee.property && node.callee.property.type === "Identifier" ? node.callee.property.name : null;
        if (objName !== "toast") return;
        if (propName !== "success" && propName !== "error" && propName !== "warning" && propName !== "info") return;

        const firstArg = node.arguments && node.arguments[0];
        if (!firstArg || firstArg.type !== "Literal") return;
        if (typeof firstArg.value !== "string") return;
        if (!isEnglishString(firstArg.value)) return;
        if (hasDisableDirective(context, node)) return;

        context.report({
          node: firstArg,
          messageId: "unlocalizedText",
        });
      },
    };
  },
};

export const rules = {
  "no-unlocalized-text": noUnlocalizedTextRule,
};