/**
 * Lo que el modo de color comparte con quien no tiene DOM: la cookie, los
 * colores de la barra y el script del `<head>`. Aparte de `theme.ts` porque
 * `vite.config.ts` lo importa y se comprueba sin los tipos del navegador.
 */

export const THEME_COOKIE = 'sello-theme'

/**
 * El `theme-color` de cada modo: la barra del navegador en el móvil. Es el
 * `--bg` de `tokens.css` en hex, porque esa etiqueta no lee variables CSS:
 * si cambia el lienzo, cambia aquí.
 */
export const THEME_COLOR = { light: '#ffffff', dark: '#100e0d' } as const

/**
 * El script del `<head>`: lo mismo que `readTheme` + `applyTheme`, sin
 * dependencias y en una línea, para ponerlo en línea antes de la hoja de
 * estilos. El chat lo inyecta con un plugin de Vite; el landing, con
 * `set:html`. El landing calcula el hash de la CSP en el build.
 */
export const THEME_SCRIPT = `(function(){try{var m=document.cookie.match(/(?:^|; )${THEME_COOKIE}=(light|dark)/),t=m?m[1]:'system',d=t==='dark'||(t==='system'&&matchMedia('(prefers-color-scheme: dark)').matches),r=document.documentElement;r.classList.toggle('dark',d);r.style.colorScheme=d?'dark':'light';var c=document.querySelector('meta[name="theme-color"]');if(c)c.setAttribute('content',d?'${THEME_COLOR.dark}':'${THEME_COLOR.light}')}catch(e){}})()`
