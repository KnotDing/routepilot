import '@testing-library/jest-dom/vitest';
// jsdom has no layout viewport; navigation tests exercise the action separately.
window.scrollTo=()=>{};
