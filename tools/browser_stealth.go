package tools

var stealthJavaScript = `
// Hide automation fingerprints
Object.defineProperty(navigator, 'webdriver', { get: () => undefined });

// Restore navigator.plugins (headless Chrome has empty plugins)
Object.defineProperty(navigator, 'plugins', {
  get: () => [
    { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer' },
    { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
    { name: 'Native Client', filename: 'internal-nacl-plugin' },
  ],
});

// Override navigator.languages to look like a real user
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en', 'zh-CN'] });

// Add a plausible chrome object
if (!window.chrome) { window.chrome = { runtime: {} }; }

// Fix permissions query to not expose automation
if (navigator.permissions && navigator.permissions.query) {
  const origQuery = navigator.permissions.query.bind(navigator.permissions);
  navigator.permissions.query = (params) => {
    if (params && (params.name === 'notifications' || params.name === 'clipboard-read')) {
      return Promise.resolve({ state: 'prompt', onchange: null });
    }
    return origQuery(params);
  };
}

// Override webgl vendor/renderer to avoid headless detection
try {
  const getParam = WebGLRenderingContext.prototype.getParameter;
  WebGLRenderingContext.prototype.getParameter = function(p) {
    if (p === 37445) return 'Intel Inc.';
    if (p === 37446) return 'Intel Iris OpenGL Engine';
    return getParam.call(this, p);
  };
} catch(e) {}

// Override screen dimensions to match viewport
try {
  Object.defineProperty(screen, 'width', { get: () => window.innerWidth });
  Object.defineProperty(screen, 'height', { get: () => window.innerHeight });
  Object.defineProperty(screen, 'availWidth', { get: () => window.innerWidth });
  Object.defineProperty(screen, 'availHeight', { get: () => window.innerHeight });
  Object.defineProperty(screen, 'colorDepth', { get: () => 24 });
  Object.defineProperty(screen, 'pixelDepth', { get: () => 24 });
} catch(e) {}
`
