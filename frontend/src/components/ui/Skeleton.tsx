import { h } from "preact";
interface SkeletonProps {
  class?: string;
  width?: string;
  height?: string;
}
export function Skeleton({
  class: className = "",
  width,
  height,
}: SkeletonProps) {
  return (
    <div
      class={`animate-pulse bg-gray-800 rounded ${className}`}
      style={{ width: width || "100%", height: height || "1rem" }}
    />
  );
}
export function DashboardSkeleton() {
  return (
    <div class="container mx-auto px-6 py-8 h-full">
      <div class="flex justify-between mb-8">
        <Skeleton width="200px" height="2rem" />
        <div class="flex space-x-3">
          <Skeleton width="120px" height="2.5rem" class="rounded-xl" />
          <Skeleton width="100px" height="2.5rem" class="rounded-xl" />
        </div>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} height="100px" class="rounded-2xl" />
        ))}
      </div>
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
        <div class="col-span-2 space-y-4">
          <Skeleton height="1.5rem" width="150px" />
          <div class="grid grid-cols-2 gap-4">
            {[1, 2, 3, 4].map((i) => (
              <Skeleton key={i} height="80px" class="rounded-2xl" />
            ))}
          </div>
        </div>
        <div class="space-y-4">
          <Skeleton height="1.5rem" width="120px" />
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} height="60px" class="rounded-2xl" />
          ))}
        </div>
      </div>
    </div>
  );
}
