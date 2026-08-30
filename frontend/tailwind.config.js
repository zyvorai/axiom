/** @type {import('tailwindcss').Config} */
module.exports = {
    darkMode: ["class"],
    content: [
    "./src/**/*.{js,jsx,ts,tsx}",
    "./public/index.html"
  ],
  theme: {
    extend: {
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)'
      },
      fontFamily: {
        sans: ['var(--font-display)'],
        mono: ['var(--font-mono)'],
      },
      fontSize: {
        // apple.com-scale headline sizes, layered on top of Tailwind's
        // defaults rather than replacing them — used by PageHeader/
        // DetailHeader and any future true marketing hero (Login, Onboarding).
        hero: ['clamp(2.75rem, 5vw + 1rem, 5.5rem)', { lineHeight: '1.02', letterSpacing: '-0.045em' }],
        display: ['clamp(2.25rem, 3vw + 1rem, 3.5rem)', { lineHeight: '1.05', letterSpacing: '-0.04em' }],
        title: ['clamp(1.75rem, 2vw + 1rem, 2.5rem)', { lineHeight: '1.1', letterSpacing: '-0.035em' }],
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
        chart: {
          '1': 'hsl(var(--chart-1))',
          '2': 'hsl(var(--chart-2))',
          '3': 'hsl(var(--chart-3))',
          '4': 'hsl(var(--chart-4))',
          '5': 'hsl(var(--chart-5))'
        },
        brand: {
          DEFAULT: 'var(--brand-hex)',
          cosmic: 'var(--i17-cosmic)',
          sky: 'var(--i17-sky-blue)',
        },
        i17: {
          cosmic: 'var(--i17-cosmic)',
          deepblue: 'var(--i17-deep-blue)',
          sky: 'var(--i17-sky-blue)',
          lavender: 'var(--i17-lavender)',
          sage: 'var(--i17-sage)',
          mist: 'var(--i17-mist)',
          white: 'var(--i17-white)',
          silver: 'var(--i17-silver)',
          gold: 'var(--i17-gold)',
        }
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