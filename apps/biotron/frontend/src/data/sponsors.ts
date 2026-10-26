import altiumLogo from '../assets/sponsors/altium.png';
import ansysLogo from '../assets/sponsors/ansys.png';
import kenestoLogo from '../assets/sponsors/kenesto.png';
import notionLogo from '../assets/sponsors/notion.png';
import odriveLogo from '../assets/sponsors/odrive.webp';
import uwSeal from '../assets/sponsors/uw-seal.png';

export type Sponsor = {
  name: string;
  /**
   * Logos are the sponsors' own files, made for a light background, so the
   * sponsors page shows them on light tiles. Do not recolour them. Only empty
   * transparent margins are trimmed. A `wordmark` spells the name and stands
   * alone. A `mark` does not read as the name at this size, so the name shows
   * beside it.
   */
  logo: { src: string; width: number; height: number; kind: 'wordmark' | 'mark' };
};

export const SPONSORS: Sponsor[] = [
  // Cropped from packages/style/src/assets/UW_Logo.png, which has wide padding.
  { name: 'University of Waterloo', logo: { src: uwSeal, width: 256, height: 256, kind: 'mark' } },
  { name: 'Notion', logo: { src: notionLogo, width: 300, height: 120, kind: 'wordmark' } },
  { name: 'Kenesto', logo: { src: kenestoLogo, width: 1200, height: 408, kind: 'wordmark' } },
  { name: 'Altium', logo: { src: altiumLogo, width: 760, height: 180, kind: 'wordmark' } },
  { name: 'ODrive', logo: { src: odriveLogo, width: 2500, height: 581, kind: 'wordmark' } },
  { name: 'Ansys', logo: { src: ansysLogo, width: 792, height: 256, kind: 'wordmark' } },
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
