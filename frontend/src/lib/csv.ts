/**
 * Escape a cell value for safe CSV export (RFC 4180 compliant with Formula Injection mitigation)
 *
 * Mitigation:
 * If the value starts with '=', '+', '-', '@', '\t', or '\r', prefix with a single quote (')
 * to prevent Excel / LibreOffice from executing formulas entered by users.
 */
export function csvCell(value: unknown): string {
  if (value === null || value === undefined) {
    return '""';
  }

  let str = String(value);

  // Formula injection prevention
  if (/^[=+\-@\t\r]/.test(str)) {
    str = `'${str}`;
  }

  // Escape double quotes by doubling them
  const escaped = str.replace(/"/g, '""');

  return `"${escaped}"`;
}
