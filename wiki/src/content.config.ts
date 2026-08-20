import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

// wiki 四象限：tutorials（学）/ how-to（做）/ reference（查）/ explanation（懂）
const wiki = defineCollection({
	loader: glob({ pattern: '**/*.md', base: './src/content/wiki' }),
	schema: z.object({
		title: z.string(),
		description: z.string(),
		section: z.enum(['tutorials', 'how-to', 'reference', 'explanation']),
		order: z.number().default(0),
		updated: z.coerce.date().optional(),
	}),
});

export const collections = { wiki };
