import altiumLogo from '../assets/sponsors/altium-white.webp';

export type Sponsor = {
  name: string;
  /** A white-on-transparent logo. Without one, the name shows as text. */
  logo?: { src: string; width: number; height: number };
};

export const SPONSORS: Sponsor[] = [
  { name: 'University of Waterloo' },
  { name: 'Notion' },
  { name: 'Kenesto' },
  { name: 'Altium', logo: { src: altiumLogo, width: 743, height: 163 } },
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
