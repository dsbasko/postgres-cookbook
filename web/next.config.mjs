// The entire Next config is the engine factory call. createCookbookConfig
// reads ../course.yaml via process.cwd() (from inside node_modules), enables
// output:'export', wires transpilePackages + the `@/*` alias for the engine
// source, and injects course.yaml.brand.siteUrl into NEXT_PUBLIC_SITE_URL.
import { createCookbookConfig } from '@dsbasko/cookbook-engine/config';

export default createCookbookConfig();
