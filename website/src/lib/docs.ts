import { getCollection, type CollectionEntry } from "astro:content";
import type { Locale } from "../content/site";

export type DocEntry = CollectionEntry<"docs">;

export interface DocNavItem {
  entry: DocEntry;
  slug: string;
  href: string;
}

export interface DocSection {
  title: string;
  order: number;
  items: DocNavItem[];
}

export function docSlugFromId(id: string, locale: Locale): string {
  const prefix = `${locale}/`;
  if (id === locale || id === `${locale}/index`) {
    return "";
  }
  const withoutLocale = id.startsWith(prefix) ? id.slice(prefix.length) : id;
  return withoutLocale.replace(/\/index$/, "");
}

export function docHref(slug: string, locale: Locale): string {
  const base = locale === "zh" ? "/zh/docs/" : "/docs/";
  return slug ? `${base}${slug}/` : base;
}

export async function getDocsByLocale(locale: Locale): Promise<DocNavItem[]> {
  const all = await getCollection("docs", ({ id, data }) => {
    const matchesLocale = id === locale || id.startsWith(`${locale}/`);
    return matchesLocale && !data.draft;
  });

  const items: DocNavItem[] = all.map((entry) => {
    const slug = docSlugFromId(entry.id, locale);
    return {
      entry,
      slug,
      href: docHref(slug, locale),
    };
  });

  items.sort((a, b) => {
    if (a.entry.data.sectionOrder !== b.entry.data.sectionOrder) {
      return a.entry.data.sectionOrder - b.entry.data.sectionOrder;
    }
    if (a.entry.data.order !== b.entry.data.order) {
      return a.entry.data.order - b.entry.data.order;
    }
    return a.entry.data.title.localeCompare(b.entry.data.title);
  });

  return items;
}

export function groupDocSections(items: DocNavItem[]): DocSection[] {
  const map = new Map<string, DocSection>();
  for (const item of items) {
    const key = `${item.entry.data.sectionOrder}:${item.entry.data.section}`;
    let section = map.get(key);
    if (!section) {
      section = {
        title: item.entry.data.section,
        order: item.entry.data.sectionOrder,
        items: [],
      };
      map.set(key, section);
    }
    section.items.push(item);
  }
  return Array.from(map.values()).sort((a, b) => a.order - b.order);
}

export async function getDocNeighbors(locale: Locale, currentSlug: string) {
  const items = await getDocsByLocale(locale);
  const index = items.findIndex((item) => item.slug === currentSlug);
  return {
    prev: index > 0 ? items[index - 1] : undefined,
    next: index >= 0 && index < items.length - 1 ? items[index + 1] : undefined,
  };
}

export async function resolveAlternateDocHref(
  targetLocale: Locale,
  slug: string
): Promise<string> {
  const items = await getDocsByLocale(targetLocale);
  const match = items.find((item) => item.slug === slug);
  if (match) return match.href;
  return targetLocale === "zh" ? "/zh/docs/" : "/docs/";
}
