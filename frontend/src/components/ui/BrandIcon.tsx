import mascot from '@/images/pigger-mascot.png';

export default function BrandIcon({ className }: { className?: string }) {
  return (
    <img src={mascot} alt="" aria-hidden="true" className={className} width={56} height={56} />
  );
}
