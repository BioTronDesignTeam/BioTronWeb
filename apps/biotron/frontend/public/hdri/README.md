# Lighting (HDRI)

The 3D scenes light themselves with these two equirectangular HDR images,
served from here so the site needs no third-party host at run time
(`src/three/lighting.ts`).

| File | Used by | Source | Licence |
|---|---|---|---|
| `empty_warehouse_01_1k.hdr` | Homepage workshop (`Scene.tsx`) | [Empty Warehouse 01](https://polyhaven.com/a/empty_warehouse_01) by Sergej Majboroda, Poly Haven | CC0 1.0 |
| `potsdamer_platz_1k.hdr` | Project pages (`ProjectViewer.tsx`) | [Potsdamer Platz](https://polyhaven.com/a/potsdamer_platz) by Greg Zaal, Poly Haven | CC0 1.0 |

Both are the 1k files drei's `warehouse` and `city` presets load, copied
byte for byte from `pmndrs/drei-assets` at commit
`456060a26bbeb8fdf79326f224b6d99b8bcce736`. Poly Haven publishes every asset
under CC0, so they may be copied, changed, and served without attribution;
the credit above is a courtesy.

SHA-256:

- `9b3d611cadc32c3a0c7e084ce5611c0650293881c4a041e7fa13748fe0dc6451` `empty_warehouse_01_1k.hdr`
- `7afe4c2f9700ee78c7477c53fa355463d7dda1fdede401432d6b5f9ff0a95696` `potsdamer_platz_1k.hdr`
