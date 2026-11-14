import { defineCollection } from "astro:content";
import { glob } from "astro/loaders";
import { z } from "astro/zod";

const docs = defineCollection({
  loader: glob({ pattern: "**/*.{md,mdx}", base: "./src/content/docs" }),
  schema: z.object({
    title: z.string(),
    description: z.string(),
    section: z.string(),
    sectionOrder: z.number().int().default(99),
    order: z.number().int().default(99),
    badge: z.string().optional(),
    draft: z.boolean().default(false),
  }),
});

export const collections = { docs };
