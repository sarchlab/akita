# Documentation website

The site uses Docusaurus and builds the tutorials and package guides from their
source Markdown in this repository. Use Node.js 22.12.0, matching CI.

## Install and develop

From `doc-site/`:

```sh
npm ci
npm start
```

## Validate and preview

```sh
npm run build
npm run serve
```

The build fails on missing sidebar document IDs, broken Markdown links,
broken site links, or missing anchors. Package documentation includes READMEs,
control guides, and DRAM roadmaps. Links between docs plugins use site routes;
links to source-only examples and release notes use GitHub URLs.

The default base path is `/akita-dev/`. To reproduce the public site:

```sh
DOCS_BASE_URL=/akita/ npm run build
DOCS_BASE_URL=/akita/ npm run serve
```

## CI and deployment

`akita_test.yml` builds the site on pull requests and pushes to `main`.
`deploy-docs.yml` builds and deploys it through GitHub Pages on matching pushes
to `main` or `v5`. Both workflows derive `DOCS_BASE_URL` from the repository
name. A passing PR build validates the site; publication happens after merge
through the separate deploy workflow.
