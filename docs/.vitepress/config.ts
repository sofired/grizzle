import { defineConfig } from 'vitepress'

// Set base to '/grizzle/' when deploying to GitHub Pages at sofired.github.io/grizzle/
// Leave as '/' for custom domain deployments.
const base = process.env.DOCS_BASE ?? '/'

export default defineConfig({
  title: 'Grizzle',
  description: 'Type-safe, code-generated query builder and migration toolkit for Go',
  base,

  head: [
    ['link', { rel: 'icon', href: `${base}favicon.ico` }],
  ],

  themeConfig: {
    nav: [
      { text: 'Guide',     link: '/guide/getting-started' },
      { text: 'Advanced',  link: '/advanced/aggregates'   },
      { text: 'Kit',       link: '/kit/overview'          },
      { text: 'Reference', link: '/reference/dialects'    },
      {
        text: 'GitHub',
        link: 'https://github.com/sofired/grizzle',
      },
    ],

    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting Started',  link: '/guide/getting-started' },
          { text: 'Schema DSL',       link: '/guide/schema'          },
          { text: 'Querying',         link: '/guide/querying'        },
          { text: 'Mutations',        link: '/guide/mutations'       },
          { text: 'Relations',        link: '/guide/relations'       },
          { text: 'Preloading',       link: '/guide/preloading'      },
          { text: 'Transactions',     link: '/guide/transactions'    },
        ],
      },
      {
        text: 'Advanced',
        items: [
          { text: 'Aggregates',       link: '/advanced/aggregates'        },
          { text: 'Window Functions', link: '/advanced/window-functions'  },
          { text: 'CASE Expressions', link: '/advanced/case-expressions'  },
          { text: 'Subqueries & CTEs',link: '/advanced/subqueries'        },
          { text: 'JSONB (PostgreSQL)',link: '/advanced/jsonb'             },
        ],
      },
      {
        text: 'Migration Kit',
        items: [
          { text: 'Overview', link: '/kit/overview' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Dialects', link: '/reference/dialects' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/sofired/grizzle' },
    ],

    editLink: {
      pattern: 'https://github.com/sofired/grizzle/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2024–present Sofired',
    },

    search: {
      provider: 'local',
    },
  },
})
