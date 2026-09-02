import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-sm font-semibold transition-[color,background-color,border-color,box-shadow,transform,filter] duration-150 focus-visible:outline-none focus-visible:shadow-[var(--focus-ring)] disabled:pointer-events-none disabled:opacity-50 active:scale-[0.97] [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        // Apple Action Blue solid fill (#0071e3) — no gradients / alternate hues
        default:
          "bg-primary text-primary-foreground shadow-[inset_0_1px_0_rgb(255_255_255_/_0.18)] hover:brightness-[0.92]",
        destructive:
          "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90",
        outline:
          "border border-input shadow-sm hover:bg-foreground/[0.06]",
        secondary:
          "bg-foreground/[0.06] text-secondary-foreground hover:bg-foreground/[0.1]",
        ghost: "hover:bg-foreground/[0.06]",
        link: "text-primary underline-offset-4 hover:underline",
        brand:
          "bg-primary text-primary-foreground shadow-[inset_0_1px_0_rgb(255_255_255_/_0.18)] hover:brightness-[0.92]",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 rounded-full px-3 text-xs",
        lg: "h-10 rounded-full px-8",
        xl: "h-12 rounded-full px-9 text-[15px]",
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
