import * as THREE from 'three';

export interface HeroScene {
  setRunning: (running: boolean) => void;
  setPointer: (x: number, y: number) => void;
  resize: () => void;
  dispose: () => void;
}

/** Self-contained decorative scene; loaded only after visibility/idle eligibility. */
export async function createHeroScene(
  host: HTMLElement,
  icons: string[],
  nucleusIcon: string,
  onFailure: () => void,
  signal: AbortSignal,
): Promise<HeroScene> {
  if (signal.aborted) throw new Error('Scene cancelled');
  const renderer = new THREE.WebGLRenderer({
    alpha: true,
    antialias: true,
    powerPreference: 'low-power',
  });
  const geometries: THREE.BufferGeometry[] = [];
  const materials: THREE.Material[] = [];
  const textures: THREE.Texture[] = [];
  const pendingImages = new Set<HTMLImageElement>();
  let disposed = false;
  let running = false;
  let textureTimeout: ReturnType<typeof setTimeout> | undefined;
  let rejectPending: ((error: Error) => void) | undefined;
  const geometry = <T extends THREE.BufferGeometry>(value: T): T => {
    geometries.push(value);
    return value;
  };
  const material = <T extends THREE.Material>(value: T): T => {
    materials.push(value);
    return value;
  };
  const canvas = renderer.domElement;
  canvas.className = 'hero-agent-field__canvas';
  const lost = (event: Event) => {
    event.preventDefault();
    dispose();
    onFailure();
  };
  canvas.addEventListener('webglcontextlost', lost);
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    clearTimeout(textureTimeout);
    rejectPending?.(new Error('Scene cancelled'));
    pendingImages.forEach((image) => {
      image.onload = null;
      image.onerror = null;
      image.src = '';
    });
    pendingImages.clear();
    signal.removeEventListener('abort', dispose);
    renderer.setAnimationLoop(null);
    canvas.removeEventListener('webglcontextlost', lost);
    geometries.forEach((value) => value.dispose());
    materials.forEach((value) => value.dispose());
    textures.forEach((value) => value.dispose());
    renderer.dispose();
    renderer.forceContextLoss();
    canvas.remove();
  };
  signal.addEventListener('abort', dispose, { once: true });

  try {
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
    renderer.setClearColor(0x000000, 0);
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(29, 2, 0.1, 40);
    camera.position.set(0, 0.6, 8.0);
    camera.lookAt(0, 0.2, 0);
    const world = new THREE.Group();
    scene.add(world);
    scene.add(new THREE.AmbientLight(0xa3b6ff, 2));
    const key = new THREE.DirectionalLight(0xffffff, 4);
    key.position.set(-3, 4, 5);
    scene.add(key);
    const rim = new THREE.PointLight(0x28d8ff, 35, 12);
    rim.position.set(2, -1, 2);
    scene.add(rim);
    const crystalGeometry = geometry(new THREE.OctahedronGeometry(0.79, 0));
    const crystal = new THREE.Mesh(
      crystalGeometry,
      material(
        new THREE.MeshPhysicalMaterial({
          color: 0x6551df,
          metalness: 0.45,
          roughness: 0.18,
          clearcoat: 1,
          transparent: true,
          opacity: 0.84,
          flatShading: true,
        }),
      ),
    );
    world.add(crystal);
    const edges = new THREE.LineSegments(
      geometry(new THREE.EdgesGeometry(crystalGeometry)),
      material(new THREE.LineBasicMaterial({ color: 0xa7e7ff, transparent: true, opacity: 0.65 })),
    );
    world.add(edges);
    const rings: THREE.Group[] = [];
    [
      { radius: 1.13, x: 0.45, y: 0.65, color: 0x8e84ff },
      { radius: 1.37, x: 1.05, y: -0.55, color: 0x51d9ef },
      { radius: 1.63, x: 1.85, y: 0.25, color: 0x777ced },
    ].forEach(({ radius, x, y, color }) => {
      const ring = new THREE.Group();
      ring.rotation.set(x, y, 0);
      ring.add(
        new THREE.Mesh(
          geometry(new THREE.TorusGeometry(radius, 0.012, 5, 96)),
          material(new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.7 })),
        ),
      );
      const spark = new THREE.Mesh(
        geometry(new THREE.SphereGeometry(0.035, 8, 6)),
        material(new THREE.MeshBasicMaterial({ color: 0xd7faff })),
      );
      spark.position.x = radius;
      ring.add(spark);
      rings.push(ring);
      world.add(ring);
    });
    const orbit = new THREE.Group();
    orbit.rotation.x = 1.03;
    orbit.rotation.z = -0.13;
    world.add(orbit);
    orbit.add(
      new THREE.Mesh(
        geometry(new THREE.TorusGeometry(2.65, 0.006, 4, 128)),
        material(
          new THREE.MeshBasicMaterial({ color: 0x99a1d4, transparent: true, opacity: 0.36 }),
        ),
      ),
    );

    const cache = new Map<string, Promise<THREE.Texture>>();
    const load = (url: string) => {
      if (!cache.has(url))
        cache.set(
          url,
          new Promise<THREE.Texture>((resolve, reject) => {
            // SVGs declare viewBox only. Rasterize at explicit dimensions instead of
            // relying on browser-dependent intrinsic SVG dimensions for GPU upload.
            const image = new Image(128, 128);
            pendingImages.add(image);
            image.onload = () => {
              pendingImages.delete(image);
              image.onload = null;
              image.onerror = null;
              if (disposed) {
                reject(new Error('Scene disposed'));
                return;
              }
              try {
                const bitmap = document.createElement('canvas');
                bitmap.width = 128;
                bitmap.height = 128;
                const context = bitmap.getContext('2d');
                if (!context) throw new Error('Logo rasterization unavailable');
                context.drawImage(image, 0, 0, 128, 128);
                const texture = new THREE.CanvasTexture(bitmap);
                texture.colorSpace = THREE.SRGBColorSpace;
                textures.push(texture);
                resolve(texture);
              } catch (error) {
                reject(error);
              }
            };
            image.onerror = () => {
              pendingImages.delete(image);
              image.onload = null;
              image.onerror = null;
              reject(new Error('Logo load failed'));
            };
            image.src = url;
          }),
        );
      return cache.get(url)!;
    };
    const loaded = await Promise.race([
      Promise.all([...icons, nucleusIcon].map(load)),
      new Promise<never>((_, reject) => {
        rejectPending = reject;
        textureTimeout = setTimeout(() => reject(new Error('Texture load timed out')), 8000);
      }),
    ]).finally(() => {
      clearTimeout(textureTimeout);
      rejectPending = undefined;
    });
    if (disposed) throw new Error('Scene cancelled');
    const badge = new THREE.Sprite(
      material(new THREE.SpriteMaterial({ map: loaded[icons.length], depthTest: false })),
    );
    badge.scale.set(0.92, 0.92, 1);
    badge.position.z = 0.83;
    world.add(badge);
    const tileGeometry = geometry(new THREE.CircleGeometry(0.28, 24));
    const tileMaterial = material(
      new THREE.MeshBasicMaterial({ color: 0xf5f7ff, side: THREE.DoubleSide }),
    );
    const nodes = icons.map((_, index) => {
      const node = new THREE.Group();
      node.add(new THREE.Mesh(tileGeometry, tileMaterial));
      const face = new THREE.Sprite(
        material(new THREE.SpriteMaterial({ map: loaded[index], depthTest: true })),
      );
      face.scale.set(0.38, 0.38, 1);
      face.position.z = 0.012;
      node.add(face);
      world.add(node);
      return node;
    });
    const orbitPosition = new THREE.Vector3();
    let elapsed = 0;
    let last = 0;
    let pointerX = 0;
    let pointerY = 0;
    let renderedFrames = 0;
    let slowFrames = 0;
    const failAfterFrame = () => {
      // Three records its next RAF after draw returns. Disposing inside draw
      // would cancel the old ID and leave a new, empty RAF loop running.
      queueMicrotask(() => {
        dispose();
        onFailure();
      });
    };
    const draw = (timestamp: number) => {
      if (disposed) return;
      if (last && timestamp - last < 1000 / 30) return;
      elapsed += last ? Math.min((timestamp - last) / 1000, 0.1) : 0;
      last = timestamp;
      world.rotation.y += (pointerX * 0.16 - world.rotation.y) * 0.045;
      world.rotation.x += (pointerY * 0.09 - world.rotation.x) * 0.045;
      crystal.rotation.y = elapsed * 0.14;
      crystal.rotation.z = 0.15 + Math.sin(elapsed * 0.25) * 0.1;
      edges.rotation.copy(crystal.rotation);
      rings.forEach((ring, index) => {
        ring.rotation.z = elapsed * (index % 2 ? -0.09 : 0.07);
      });
      nodes.forEach((node, index) => {
        const angle = (index * Math.PI * 2) / nodes.length + elapsed * 0.07;
        orbitPosition
          .set(Math.cos(angle) * 2.65, Math.sin(angle) * 2.65, 0)
          .applyEuler(orbit.rotation);
        node.position.copy(orbitPosition);
        node.quaternion.copy(camera.quaternion);
      });
      try {
        const started = performance.now();
        renderer.render(scene, camera);
        const renderCost = performance.now() - started;
        renderedFrames++;
        // Ignore initial shader/upload work; only sustained synchronous render
        // pressure disables this optional enhancement for the component lifetime.
        if (renderedFrames > 5) {
          slowFrames = renderCost > 20 ? slowFrames + 1 : 0;
          if (slowFrames >= 20) {
            failAfterFrame();
          }
        }
      } catch {
        failAfterFrame();
      }
    };
    const resize = () => {
      if (disposed) return;
      const width = host.clientWidth;
      const height = host.clientHeight;
      if (!width || !height) return;
      renderer.setSize(width, height, false);
      camera.aspect = width / height;
      camera.position.z = Math.max(8.0, 10.1 / camera.aspect);
      camera.updateProjectionMatrix();
      if (!running) draw(performance.now());
    };
    // Compile on the first idle-scheduled draw. Three's compileAsync readiness
    // polling cannot be cancelled safely when this optional scene is disposed.
    if (disposed) throw new Error('Scene cancelled');
    host.appendChild(canvas);
    resize();
    return {
      resize,
      dispose,
      setPointer(x, y) {
        pointerX = x;
        pointerY = y;
      },
      setRunning(value) {
        if (disposed || running === value) return;
        running = value;
        last = 0;
        slowFrames = 0;
        renderer.setAnimationLoop(value ? draw : null);
      },
    };
  } catch (error) {
    dispose();
    throw error;
  }
}
