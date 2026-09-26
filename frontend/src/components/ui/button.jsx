import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  // Netra: pill, 400 weight, 0.28s apple ease, solid (never faded) disabled fill.
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-normal tracking-[-0.016em] transition-[color,background-color,border-color,box-shadow,transform,filter] duration-300 ease-apple focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-outline)] disabled:pointer-events-none disabled:border-transparent disabled:bg-[var(--disabled-bg)] disabled:text-[var(--disabled-text)] disabled:shadow-none active:scale-[0.98] [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        // Action Blue #0071e3; dark-mode hover adds the blue glow (netra)
        default:
          "bg-primary text-primary-foreground hover:brightness-[1.06] dark:hover:shadow-[0_8px_24px_rgba(0,113,227,0.35)]",
        destructive:
          "bg-destructive text-destructive-foreground hover:brightness-[1.08]",
        outline:
          "border border-[var(--line-2)] bg-transparent hover:bg-accent",
        secondary:
          "bg-accent text-foreground hover:bg-[var(--hover-fill-strong)]",
        ghost: "hover:bg-accent",
        link: "text-tone-link underline-offset-4 hover:underline",
        brand:
          "bg-primary text-primary-foreground hover:brightness-[1.06] dark:hover:shadow-[0_8px_24px_rgba(0,113,227,0.35)]",
      },
      size: {
        default: "h-9 px-5 py-2",
        sm: "h-8 px-3.5 text-[13px]",
        lg: "h-11 px-6 text-[17px] tracking-[-0.022em]",
        xl: "h-12 px-9 text-[17px] tracking-[-0.022em]",
        icon: "h-9 w-9",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

const Button = React.forwardRef(({ className, variant, size, asChild = false, ...props }, ref) => {
  const Comp = asChild ? Slot : "button"
  return (
    <Comp
      className={cn(buttonVariants({ variant, size, className }))}
      ref={ref}
      {...props} />
  );
})
Button.displayName = "Button"

export { Button, buttonVariants }
