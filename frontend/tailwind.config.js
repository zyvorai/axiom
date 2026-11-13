/** @type {import('tailwindcss').Config} */
module.exports = {
    darkMode: ["class"],
    content: [
    "./src/**/*.{js,jsx,ts,tsx}",
    "./public/index.html"
  ],
  theme: {
    extend: {
      // Netra radius scale (styles/tokens.css). Cards = 3xl, tiles = xl,
      // inputs = md, buttons/badges = full.
      borderRadius: {
        xs: 'var(--radius-xs)',
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        '2xl': 'var(--radius-2xl)',
        '3xl': 'var(--radius-3xl)',
      },
      boxShadow: {
        card: 'var(--card-shadow)',
        pop: 'var(--pop-shadow)',
      },
      transitionTimingFunction: {
        apple: 'var(--ease-apple)',
      },
      fontFamily: {
        sans: ['var(--font-display)'],
        hero: ['var(--font-hero)'],
        mono: ['var(--font-mono)'],
      },
      fontSize: {
        // Console-scale headlines (not marketing-billboard sizes).
        hero: ['clamp(1.75rem, 2.5vw + 0.5rem, 2.5rem)', { lineHeight: '1.08', letterSpacing: '-0.035em' }],
        display: ['clamp(1.5rem, 2vw + 0.4rem, 2rem)', { lineHeight: '1.1', letterSpacing: '-0.03em' }],
        title: ['clamp(1.25rem, 1.5vw + 0.35rem, 1.6rem)', { lineHeight: '1.15', letterSpacing: '-0.025em' }],
      },
      colors: {
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        card: {
          DEFAULT: 'hsl(var(--card))',
          foreground: 'hsl(var(--card-foreground))'
        },
        popover: {
          DEFAULT: 'hsl(var(--popover))',
          foreground: 'hsl(var(--popover-foreground))'
        },
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))'
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary))',
          foreground: 'hsl(var(--secondary-foreground))'
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))'
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))'
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))'
        },
        success: 'hsl(var(--success))',
        warning: 'hsl(var(--warning))',
        info: 'hsl(var(--info))',
        danger: 'hsl(var(--danger))',
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        // Netra accents/soft surfaces (styles/tokens.css) — use these instead of
        // raw palette classes (text-rose-400, bg-white/5 …). Plain var() colours,
        // so no /opacity modifier: use the -soft surfaces for tints.
        line: 'var(--line)',
        // Shell text ramp (--ink*) for pages that render outside shadcn tokens
        ink: {
          DEFAULT: 'var(--ink)',
          '2': 'var(--ink-2)',
          '3': 'var(--ink-3)',
          '4': 'var(--ink-4)',
        },
        tone: {
          blue: 'var(--apple-blue)',
          link: 'var(--apple-link)',
          cyan: 'var(--accent-cyan)',
          green: 'var(--green)',
          amber: 'var(--amber)',
          orange: 'var(--orange)',
          red: 'var(--red)',
          purple: 'var(--purple)',
          teal: 'var(--teal)',
        },
        // Terminal / code panes — dark in both themes (--terminal-* tokens)
        term: {
          bg: 'var(--terminal-bg)',
          bar: 'var(--terminal-bar)',
          text: 'var(--terminal-text)',
          dim: 'var(--terminal-text-dim)',
          hover: 'var(--terminal-hover)',
          key: 'var(--tok-key)',
          str: 'var(--tok-str)',
          num: 'var(--tok-num)',
          kw: 'var(--tok-kw)',
          punct: 'var(--tok-punct)',
          plain: 'var(--tok-plain)',
        },
        soft: {
          danger: 'var(--danger-bg)',
          'danger-border': 'var(--danger-border)',
          warning: 'var(--warning-bg)',
          'warning-border': 'var(--warning-border)',
          'warning-text': 'var(--warning-text)',
          success: 'var(--success-bg)',
          'success-border': 'var(--success-border)',
          info: 'var(--info-bg)',
          'info-border': 'var(--info-border)',
        },
      },
      keyframes: {
        'accordion-down': {
          from: {
            height: '0'
          },
          to: {
            height: 'var(--radix-accordion-content-height)'
          }
        },
        'accordion-up': {
          from: {
            height: 'var(--radix-accordion-content-height)'
          },
          to: {
            height: '0'
          }
        }
      },
      animation: {
        'accordion-down': 'accordion-down 0.2s ease-out',
        'accordion-up': 'accordion-up 0.2s ease-out'
      }
    }
  },
  plugins: [require("tailwindcss-animate")],
};