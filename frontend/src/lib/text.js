/**
 * Adds invisible break opportunities (zero-width spaces) after "/", ":" and "."
 * so long refs like "service:commerce/checkout" or "monitoring.coreos.com/v1"
 * wrap at their separators instead of mid-word or by scrolling sideways.
 */
export const breakable = (v) => String(v ?? "").replace(/([/:.])/g, "$1​");
