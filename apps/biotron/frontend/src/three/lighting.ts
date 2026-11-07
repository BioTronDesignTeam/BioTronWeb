/**
 * The image-based lighting the scenes use, served from this site's own
 * public/hdri folder. drei's `preset` names fetch the same files from
 * raw.githack.com at run time, which the edge's CSP does not allow and which
 * made the whole homepage depend on a third-party host. Licences are in
 * public/hdri/README.md.
 */
const base = `${import.meta.env.BASE_URL}hdri/`;

export const HDRI = {
  /** drei's "city" preset: Poly Haven's Potsdamer Platz. */
  city: `${base}potsdamer_platz_1k.hdr`,
  /** drei's "warehouse" preset: Poly Haven's Empty Warehouse 01. */
  warehouse: `${base}empty_warehouse_01_1k.hdr`,
} as const;
