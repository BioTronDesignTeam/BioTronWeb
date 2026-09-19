import altiumLogo from '../assets/sponsors/altium-white.webp';
import uwSeal from '../assets/sponsors/uw-seal.png';

export type Sponsor = {
  name: string;
  /**
   * Without a logo, the name shows as text. A `wordmark` spells the name, so it
   * stands alone; it must be white on transparent. A `mark` is a symbol that
   * does not read as the name at small sizes, so the name shows beside it.
   */
  logo?: { src: string; width: number; height: number; kind: 'wordmark' | 'mark' };
};

export const SPONSORS: Sponsor[] = [
  // Cropped from packages/style/src/assets/UW_Logo.png, which has wide padding.
  { name: 'University of Waterloo', logo: { src: uwSeal, width: 256, height: 256, kind: 'mark' } },
  { name: 'Notion' },
  { name: 'Kenesto' },
  { name: 'Altium', logo: { src: altiumLogo, width: 743, height: 163, kind: 'wordmark' } },
  { name: 'ODrive' },
  { name: 'Ansys' },
];

export const SPONSORSHIP_AREAS = [
  {
    title: 'Build better prototypes',
    body: 'Fund the parts, materials, software, and shop access we need to turn concepts into tested systems.',
  },
  {
    title: 'Develop engineers',
    body: 'Give students hands-on experience across mechanical, electrical, software, controls, and biomedical engineering.',
  },
  {
    title: 'Extend our reach',
    body: 'Help us deliver assistive devices, reach competition milestones, and work with more community partners.',
  },
];
