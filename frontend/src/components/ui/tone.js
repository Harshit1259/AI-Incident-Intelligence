/**
 * The tone vocabulary.
 *
 * One name per meaning — danger, warning, success, info, brand, neutral —
 * resolved to a CSS class that carries the matching token set. Components and
 * pages pass a tone name and never a colour, which is what keeps four
 * different reds from creeping back in.
 *
 * Lives in its own module rather than beside the components so the primitives
 * file exports components only (React Fast Refresh requires that).
 */
const TONES = ["danger", "warning", "success", "info", "brand", "neutral"];

export const toneClass = (tone) => `tone-${TONES.includes(tone) ? tone : "neutral"}`;

/* Severity and status both collapse onto tones. Defined once here so no page
   has to remember that "high" is orange and "acknowledged" is amber. */
export const SEVERITY_TONE = {
  critical: "danger",
  high: "warning",
  medium: "warning",
  low: "success",
  info: "info",
  // No-data / query-error alerts: the check failed, so severity is not known.
  unknown: "neutral",
};

export const STATUS_TONE = {
  open: "danger",
  acknowledged: "warning",
  resolved: "success",
  closed: "success",
  merged: "neutral",
};
