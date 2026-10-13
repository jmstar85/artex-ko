# api-recon — 참조 매뉴얼

Grep 레시피, `config.json` 템플릿, 트러블슈팅. 모든 grep은 `js/` 디렉터리를 대상으로 실행한다. bundle이 한 줄로 되어 있을 때는 먼저 `js-beautify` 또는 `sed 's/}/}\n/g'`를 적용할 수 있으며, 보통은 컨텍스트 윈도우를 포함한 raw grep만으로도 충분하다.

## 스크립트 설명

`scripts/` 내의 모든 파일은 **참조 템플릿**이며, 실행 전 반드시 대상 사이트에 맞게 조정해야 한다. 전형적인 조정 포인트:

| 스크립트 | 흔히 조정이 필요한 항목 |
|---|---|
| `harvest_static.py` | endpoint 정규식, webpack/Vite manifest 파싱, 마이크로 프론트엔드 publicPath, 재시도/동시성 |
| `runtime_harvest.js` | neutralize 필드명과 성공 값, stub 매칭 규칙과 body 구조, routes 출처, WS 녹화, `waitUntil`/`routeTimeout`/`proxy` |
| `preload.js` | `loginPathRe`, L1 stubs, `neutralize.fields`, `apiPattern`, L3 활성화 여부, `recordDetail`, `observe.*`, `neutralizeVueRouter` |
| `spider_mpa.py` | `--exclude` 파괴적 링크, cookie, depth/max, 동일 도메인 필터 |
| `extract_route_map.py` | `routeMap` / `routeLink` 정규식, KEY 네이밍 패턴 |
| `build_perm_tree.py` | `userRouteAuth` 파싱, `ROOTS`/`PREFIX_PARENT` 계층 휴리스틱, stub 외곽 필드명 |
| `config.json` | 위의 모든 사이트 전용 파라미터를 통합하는 진입점 |

조정한 파일은 작업 디렉터리(예: `recon/`)에 두는 것을 권장하며, 리포트에는 참조 스크립트 대비 구체적인 변경 사항을 명시한다.

---

## A. 리버싱 삼중 게이트

### A1. 렌더링 게이트 — 「로그인 여부를 어떻게 판단하는가?」

```bash
grep -rhoaE '.{0,40}(isLogin|isAuthenticated|loggedIn|hasLogin|requireAuth)\b.{0,80}' js | head
grep -rhoaE 'function (getUser|getToken|getAuth)[0-9]?\([^)]*\)\{.{0,200}' js | head
grep -rhoaE '(localStorage|sessionStorage)\.getItem\("[^"]+"\)' js | sort -u
grep -rhoaE '(Cookies?|cookie)\.(get|load)\("[^"]+"\)' js | sort -u
grep -rhoaE '\batob\(|JSON\.parse\(|jwt|decode' js | head
```

`isLogin = f(getUser())` → `getUser = decode(storage.read(KEY))` 형태의 체인을 찾아 **저장 키**, **컨테이너**(Cookie vs localStorage), **인코딩**을 확정한다:

| 인코딩 | config 위조 방식 |
|---|---|
| 평문 문자열 / `"1"` / token | `"value": "anything-truthy"` |
| `JSON.parse(x)` | `"value": "json:{\"id\":1,\"username\":\"admin\"}"` |
| `JSON.parse(atob(x))` | `"value": "b64json:{\"id\":1,\"username\":\"admin\"}"` |
| JWT | 서명 없는/`alg:none` JWT, 또는 bundle 내 키로 서명 |
| 암호화(SM2/AES/RSA) | 하드코딩된 키를 찾는다; 렌더링 게이트만 디코딩 가능한 blob이면 forge 가능; 아니면 정적 분석으로 대체 |

→ `cookies` / `localStorage`에 기록한다.

### A2. 인터셉터 게이트 — 「무엇이 /login으로의 점프를 유발하는가?」

```bash
grep -rhoaE '.{0,60}(interceptors\.response|axios|request\.use).{0,120}' js | head
grep -rhoaE '.{0,40}(response_code|errcode|errno|\bcode\b|\bret\b|\bstatus\b)\s*[=!]==?\s*[\-0-9]{1,4}.{0,60}' js | head -20
grep -rhoaE '.{0,40}(未登录|请重新登录|登录已过期|unauthorized|登录失效|授权|token.{0,10}invalid).{0,40}' js | head
grep -rhoaE '.{0,30}(location\.href|router\.(push|replace)|navigate)\([^)]*login[^)]*\)' js | head
```

다음을 확정한다: **필드명**, **성공 값**(보통 `0` 또는 `200`), **점프를 유발하는 실패 값**. junk 세션으로 검증한다:

```bash
curl -sk -X POST -H 'Cookie: <fakekey>=junk' https://target/api/<protected> -d '{}' -H 'Content-Type: application/json'
```

→ `neutralize.fields` + `neutralize.success`에 기록한다.

### A3. 콘텐츠 게이트 — 「메뉴/권한은 어디서 오는가?」

```bash
grep -rhoaE '"/api[^"]*(permission|perm|role|menu|acl|resource|nav)[^"]*"' js | sort -u
grep -rhoaE '.{0,30}(menus|permissions|menuList|routeList|authList|role_permissions)\b.{0,120}' js | head
grep -rhoaE 'userRouteAuth|getResultTree|routeMap|routeLink|hasPermission|checkAuth' js | head
grep -rhoaE '([A-Z_][A-Z0-9_]*):\{name:"[^"]*",link:"/[^"]+"\}' js | head
```

**두 계층의 데이터**(흔한 기업용 백오피스 패턴):

| API | 전형적인 payload | 소비 주체 |
|---|---|---|
| `.../role_permissions` | `{ permissions: string[], role_type }` | 라우트 가드, 버튼 단위 ACL |
| `.../permissions/all` | `tree[{ code, position, children }]` | 사이드바 메뉴 렌더링 |
| bundle 내 `userRouteAuth` | `{ CODE: { url, name? } }` | code → 프런트엔드 path |
| bundle 내 `routeMap` | `{ KEY: { name, link } }` | 별칭 해석(webpack `o.DASHBOARD`) |

소비 측 코드를 읽어 확인한다: `getResultTree(tree, permissions)`가 어떻게 필터링하는지, `v-if` / `hasAuth(code)`가 어떤 필드를 검사하는지.

**수동 forge**(소규모 사이트): permissive payload를 구성 → `stubs`.

**완전한 권한 트리 복원**(대규모 사이트, 사이드바/하위 모듈이 여전히 비어 있을 때): **I절** 참조.

---

## B. config.json 템플릿

```json
{
  "baseUrl": "https://target/",
  "runtimeMode": "both",
  "chromium": "/usr/bin/chromium",

  "cookies": [
    { "name": "auth", "value": "b64json:{\"id\":1,\"username\":\"admin\",\"role\":\"admin\",\"func\":{},\"permissions\":[\"*\"]}" }
  ],
  "localStorage": { "token": "faketoken", "isLogin": "1" },

  "neutralize": {
    "fields": ["response_code", "code", "errno", "ret", "status"],
    "success": 0,
    "flags": { "success": true, "message": "ok" }
  },
  "forward": true,
  "loginUrlPattern": "/login",
  "apiPattern": "/api/|/rest/|/graphql",

  "mockTier": "L1+L2",
  "recordDetail": true,
  "observe": {
    "storageReads": false,
    "cookieReads": false,
    "xhrHeaders": true
  },
  "neutralizeVueRouter": true,
  "stubs": [
    {
      "match": "permissions/all|/menu|role_permissions",
      "body": {
        "response_code": 0, "code": 0,
        "data": {
          "permissions": ["*"],
          "menus": [
            { "name": "dashboard", "path": "/dashboard", "show": true, "children": [] },
            { "name": "alert", "path": "/alert", "show": true, "children": [] }
          ]
        }
      }
    }
  ],

  "explore": {
    "clickTabs": true,
    "clickTables": true,
    "pushStateFallback": true,
    "maxMenuItems": 50
  },

  "routes": ["/dashboard", "/alert", "/asset", "/device", "/report", "/config", "/system"],
  "waitMs": 1500, "perRouteMs": 900, "headless": true,
  "waitUntil": "domcontentloaded",
  "routeTimeout": 12000,
  "proxy": "",

  "captureResponses": true, "recordWs": true, "respMax": 600
}
```

필드 설명:
- `runtimeMode`: `depth`(Puppeteer), `coverage`(browser MCP), `both`
- `cookies[].value` 접두사: `b64json:` → base64(JSON); `json:` → 원시 JSON; 접두사 없음 → 리터럴
- `forward: true`는 실제 요청을 전달하며 코드 필드를 재작성; `false`는 완전 오프라인 stub
- `mockTier`: coverage 모드 preload에서 활성화할 계층, 예: `L1+L2`, `L1+L2+L3`
- `routes`는 `routes.txt`에서 가져오며, 메뉴를 forge한 후 harness가 `<a href>`를 자동으로 추가한다
- `captureResponses` / `recordWs`는 depth 모드에서만 유효
- `waitUntil`: 대형 SPA에는 `domcontentloaded`를 사용하여 `networkidle2`의 행(hang) 문제를 피한다
- `routeTimeout`: 단일 라우트의 `page.goto` 타임아웃(밀리초)
- `proxy`: Puppeteer `--proxy-server`; `HTTP_PROXY` / `HTTPS_PROXY` 환경 변수로도 설정 가능

### B1. 이중 stub 템플릿(role_permissions + permissions/all)

```json
"stubs": [
  {
    "match": "role_permissions",
    "body": {
      "response_code": 0,
      "data": {
        "permissions": ["MONITOR", "MONITOR_ALERT", "THREAT", "ASSETS_RISK"],
        "role_type": "SUPER_ADMIN"
      }
    }
  },
  {
    "match": "permissions/all",
    "body": {
      "response_code": 0,
      "data": [
        {
          "code": "MONITOR",
          "position": 1,
          "children": [
            { "code": "MONITOR_ALERT", "position": 1, "children": [] }
          ]
        }
      ]
    }
  }
]
```

외곽 필드명(`response_code` / `code` / `data`)은 A2 인터셉터 게이트와 일치해야 한다; `permissions`는 tree 내 모든 leaf code를 커버해야 한다.

---

## C. coverage 모드: preload 설정

`scripts/preload.js` 상단의 `CONFIG` 객체를 편집하거나, CDP 주입 전에 교체한다:

```javascript
const CONFIG = {
  loginPathRe: /\/(login|signin)(\/|$|\?)/i,
  mockTier: 'L1+L2',
  forward: true,
  recordDetail: true,
  extractUrlsFromResponse: true,
  neutralizeVueRouter: true,
  observe: { storageReads: false, cookieReads: false, xhrHeaders: true },
  neutralize: { fields: ['response_code', 'code'], success: 0 },
  stubs: [ /* config.json의 stubs와 동일 */ ],
  apiPattern: /\/(api|apis|v\d+|dev|internal|graphql)\//i,
};
```

검증: `window.__API_RECON_PRELOAD__ === true`이며 pathname이 안정적일 것.

녹화 결과 내보내기:

```javascript
JSON.stringify({
  apis: [...window.__API_RECON_LOG__],
  detail: window.__API_RECON_DETAIL__,
  routes: [...(window.__API_RECON_ROUTES__ || [])],
  observe: window.__API_RECON_OBSERVE__,
}, null, 2)
```

---

## D. preload / runtime Hook 역량

preload(coverage)와 runtime_harvest(depth)에 내장된 브라우저 Hook 역량 및 커버리지:

| Hook 역량 | API 발견에 대한 가치 | 커버리지 |
|---|---|---|
| Hook fetch / XHR.open | 요청 URL/메서드 기록 | ✅ `recordDetail` + `__API_RECON_LOG__` |
| Hook XHR.setRequestHeader | Authorization 등 헤더 발견 | ✅ `observe.xhrHeaders` |
| localStorage/cookie 읽기 Hook | 세션 키 이름 확인 | ⚠️ 선택적 `observe.storageReads/cookieReads` |
| Vue 라우트 획득 | frontendRoutes 보완 | ✅ `__API_RECON_ROUTES__`(이미 로드된 라우트) |
| Vue 라우트 가드 무력화 / 로그인 점프 차단 | 모듈을 펼쳐 API 트리거 | ✅ `neutralizeVueRouter` + 네이티브 점프 무력화 |
| React 라우트 획득 | 라우트 보완 | ⚠️ 정적 + 클릭; 전용 Hook 없음 |
| 페이지 점프 차단(로그인 path) | 페이지에 머물며 분석 | ⚠️ 로그인 path만 차단하여 비즈니스 내비게이션을 막지 않음 |
| 암호화 라이브러리 Hook(CryptoJS/SM 등) | 암호화된 파라미터 → 평문 API body | ❌ 암호화 함수의 입력 파라미터를 수동 Hook 해야 함; 결론은 config에 기록 |
| 안티 디버깅 우회 | 그렇지 않으면 runtime이 API를 기록하지 못함 | ❌ 수동 처리 필요; 정적 분석은 여전히 사용 가능 |

---

## E. Endpoint 추출 정규식(정적 결과가 부족할 때)

`harvest_static.py`의 `extract_endpoints`를 완화하거나, 수동으로:

```bash
grep -rhoaE '"/[a-z][A-Za-z0-9_/\-]{3,}"' js | sort -u
grep -rhoaE '/api/[a-zA-Z0-9_./-]+' js | sort -u
```

---

## F. 트러블슈팅

| 현상 | 원인 → 처리 |
|---|---|
| 정적 API가 매우 적음 | endpoint 방언이 맞지 않음 → 정규식을 완화(D절) |
| chunk 수 ≪ manifest | CSS-only 또는 배포되지 않은 chunk; 404는 이미 재시도됨 |
| runtime이 여전히 로그인 페이지를 표시 | 렌더링 게이트 오류 → A1 재검토: 키 이름, 컨테이너, 인코딩, domain |
| 셸에는 진입했지만 모듈이 비어 있음 | 콘텐츠 게이트 → 메뉴 forge(A3); `routes` path가 잘못됐을 수 있음 |
| 각 라우트에 bootstrap/locale만 있음 | 권한 코드가 불완전 → I절 권한 트리 복원; `role_permissions` + `permissions/all` 이중 stub 확인 |
| 사이드바에는 항목이 있지만 하위 페이지가 비어 있음 | tree에 중간 노드가 빠졌거나 code가 `userRouteAuth`와 불일치 |
| 모든 API가 로그인으로 점프 | 인터셉터 게이트 → `neutralize` 확인; 중첩 필드는 walk 로직 확장 필요 |
| WS 프레임이 0 | 사용자 상호작용 후에만 subscribe됨; `perRouteMs`를 늘림 |
| 응답 바디가 비어 있음 | `forward: true`일 때만 실제 응답이 존재 |
| Chromium 누락 | chromium 설치 또는 `config.chromium` / `CHROMIUM` 설정 |
| Mock이 많은데도 로그인으로 돌아감 | Hook이 너무 늦었거나 `location.href` setter 누락 → document-start + preload |
| 리스트가 전부 비어 있음 | L3 빈 배열은 정상; Tab/설정/상세를 계속 클릭 |
| Redux action을 라우트로 오인 | get/set/change/clear/toggle/upload를 포함한 내부 path는 필터링 |
| Vue가 여전히 로그인으로 점프 | preload가 document-start가 아님 → 주입 시점 변경; 또는 `neutralizeVueRouter: false`일 때 수동으로 가드 제거 |
| 응답에 URL이 있지만 log에 들어가지 않음 | `extractUrlsFromResponse`를 켬; 또는 `__API_RECON_DETAIL__`에서 수동 추출 |
| Authorization 헤더 이름을 모름 | `observe.xhrHeaders`를 켜거나 DevTools에서 요청 헤더 확인 |
| runtime이 매우 느림 / 타임아웃 | `waitUntil: domcontentloaded`로 변경; `routeTimeout`을 낮춤; `networkidle2`는 사용하지 않음 |
| 프록시 연결 실패 | `proxy` / 환경 변수 확인; Puppeteer와 curl의 프록시 포트가 일치해야 함 |

---

## G. hardened 대상

서버 측이 세션을 단계적으로 검증(위조 불가능한 서명된 cookie, 서버 사이드 렌더링되어 stub할 수 없는 메뉴)할 경우, runtime은 셸 단계에서 멈춘다. 예상되는 동작:

- **정적 분석만으로 endpoint 열거는 충분** — 모듈 path는 코드 안에 있다
- 권한이 허용된다면, **실제 세션**으로 동일한 harness를 실행: `forward: true`, neutralize 불필요, 실제 methods/params/responses를 캡처

---

## H. 단일 작업 체크리스트

1. 권한 범위 확인
2. `scripts/harvest_static.py`를 **읽고** → 대상에 맞게 조정 → 실행 → `api_static.txt`, `routes.txt` 검토
3. **Phase 1b**: path 앵커 확장 윈도우 + 바인딩 계층 → `param_candidates.json`(J절)
4. A1/A2/A3 리버싱 → 사이트 전용 `config.json` 작성
5. `runtime_harvest.js` / `preload.js`를 **읽고 조정한 후** 실행
6. `runtimeMode=depth`: `npm install` → 조정한 harvest 스크립트 실행
7. `runtimeMode=coverage/both`: document-start에 조정한 preload 주입 → browser MCP 동적 열거 + **파라미터 트리거 매트릭스**
8. 모듈이 렌더링되지 않으면 → **I절 권한 트리 복원** → stub patch → 재실행
9. 파라미터 다중 샘플 diff + 오류 역추론 → `params_merged.json`
10. 병합 → `site_map.json` + `api_merged.txt`, 커버리지/갭/스크립트 변경 사항을 정직하게 명시

---

## I. 권한 트리 복원(Phase 4 심화)

단순한 `menus: [{ path, show: true }]` forge가 효과가 없고, 하위 모듈이 여전히 mount되지 않을 때 사용한다.

### I1. auth 모듈 위치 찾기

```bash
grep -l 'userRouteAuth' js/*.js
grep -l 'routeMap\|routeLink' js/*.js
grep -rhoaE 'getResultTree|role_permissions|permissions/all' js | head
```

기록할 것: **권한 API path**, **응답 필드명**, **소비 chunk 파일명**.

### I2. routeMap 추출

```bash
python3 scripts/extract_route_map.py recon/js recon/
# 결과물 recon/route_map.json
```

`[!] no routeMap pattern found`인 경우: `extract_route_map.py`의 정규식을 완화하거나, 수동으로 grep:

```bash
grep -rhoaE '([A-Z_][A-Z0-9_]*):\{name:"[^"]*",link:"/[^"]+"\}' js | head -20
```

### I3. 권한 트리 + stub 구축

```bash
python3 scripts/build_perm_tree.py recon/js recon/ --config recon/config.json
```

스크립트 로직:
1. `userRouteAuth={MONITOR:{url:...},...}`을 파싱(webpack 별칭 `He=o.DASHBOARD` 포함)
2. `route_map.json`을 사용해 alias → 실제 path를 해석
3. code 접두사에 따라 parent를 추론(`MONITOR_ALERT` → `MONITOR`)
4. `permissions_tree.json`, `permissions_all_stub.json`, `role_permissions_stub.json`을 출력
5. `--config`가 있으면 `config.json`의 `stubs`와 확장된 `routes`를 자동으로 기록

**대상에 맞게 조정**(스크립트 상단):
- `DEFAULT_ROOTS`: 최상위 모듈 code 목록
- `DEFAULT_PREFIX_PARENT`: `PREFIX_` → parent 매핑
- `DEFAULT_EXTRA_PARENT`: 접두사 관계가 아닌 orphan 노드

### I4. stub 일관성 검증

```bash
# permissions 수는 userRouteAuth 항목 수와 거의 일치해야 함
wc -l recon/perm_codes_all.txt
# routes는 route_map의 모든 link를 커버해야 함
python3 -c "import json; m=json.load(open('recon/route_map.json')); r=set(json.load(open('recon/config.json'))['routes']); print('missing', [v['link'] for v in m.values() if v['link'] not in r])"
```

### I5. runtime 재실행 및 비교

```bash
node recon/runtime_harvest.js recon/config.json
# forge 전후 runtime_api.json 건수를 비교; /attack, /asset 등 모듈 API가 나타나는지 확인
```

| forge 전 | forge 후(성공) |
|---|---|
| 모든 라우트에서 동일한 3–5개 bootstrap | 서로 다른 라우트가 서로 다른 모듈 API를 트리거 |
| `/api/locale/language`만 존재 | `/api/web/...` 모듈 endpoint가 나타남 |
| `routes.txt`에 라우트가 몇 개뿐 | `routes`가 route_map에서 가져온 80–110개 이상 |

### I6. 여전히 실패할 때

- **coverage 모드**: 사이드바 + Tab을 클릭; 권한 gating이 상호작용 이후에만 요청될 수 있음
- **stub 필드**: 실제 API(curl + 실제 session)와 stub의 중첩 구조를 비교
- **추가 가드**: `hasPermission|checkRole|func.` 등 버튼 단위 검사를 grep하여 `role_permissions.permissions`를 확장
- **정적 분석 대체**: 모듈 API path는 여전히 `api_static.txt`에 있으며, runtime은 METHOD/body만 보완; 파라미터는 `param_candidates.json` + 기록된 샘플을 유지

---

## J. 파라미터 리버싱(Phase 1b / 5b / 5c)

**방법론이지, 범용 스크립트가 아니다.** path를 찾을 때는 정규식을 사용하고; 파라미터를 찾을 때는 앵커 확장 윈도우 + UI 바인딩 체인 + 다중 샘플 diff + 오류 역추론을 사용한다.

### J1. 앵커 확장 윈도우 — path로부터 조합 객체 찾기

```bash
# Phase 1에서 이미 알려진 path를 앵커로 사용
grep -n '"/api/user/list"' js/*.js
grep -rhoaE '.{0,120}("/api[^"]+").{0,200}' js | head
grep -rhoaE '(params|data|body|payload)\s*:\s*\{' js | head
grep -rhoaE '(get|post|put|delete|patch)\([^,]+,\s*\{' js | head
```

### J2. 래핑 계층과 전송 형태

```bash
# axios / 통합 request
grep -rhoaE '(axios|request)\.(get|post|put|delete|patch)\(' js | head
grep -rhoaE 'interceptors\.(request|response)' js | head

# GraphQL
grep -rhoaE '(query|mutation)\s+\w+|gql`|graphql\(' js | head
grep -rhoaE '\$[a-zA-Z_]+\s*:\s*(Int|String|Boolean|\[)' js | head

# FormData / multipart
grep -rhoaE 'FormData|\.append\(' js | head

# 경로 파라미터
grep -rhoaE 'path:\s*"/[^"]*:[^"]+"' js | head
grep -rhoaE 'useParams|route\.params|\$route\.params' js | head
```

### J3. 검증 게이트 — 필수 / 형식 / 열거형

```bash
grep -rhoaE '(required|message|pattern|enum|validator)\s*:' js | head
grep -rhoaE 'yup\.|zod\.|async-validator|Form\.Item|a-form-item|el-form-item' js | head
grep -rhoaE 'rules\s*:\s*\[|name:\s*["\'][a-zA-Z_]+["\']' js | head
grep -rhoaE 'label.*value|options\s*:\s*\[' js | head
```

### J4. 바인딩 계층 — 폼 → API

```bash
grep -rhoaE 'onFinish|handleSubmit|getFieldsValue|validateFields' js | head
grep -rhoaE '(pick|omit|transform|dayjs|moment)\(' js | head
```

runtime 보완: DevTools → Network → 요청 → **Initiator**(call stack)에서 `fetch`/`send`로부터 위로 거슬러 올라가며 조합 함수를 추적한다.

### J5. 암호화된 파라미터

```bash
grep -rhoaE 'encrypt|decrypt|sign|CryptoJS|sm2|sm3|sm4|RSA|AES' js | head
```

**암호문을 보고 필드를 추측하지 말 것** — 암호화 함수의 **입력 파라미터**를 Hook하여, 암호화 전의 plaintext payload를 기록한다; 결론은 `config.json` / `param_candidates.json`에 기록한다.

### J6. 파라미터 트리거 매트릭스(Phase 3 필수)

각 모듈에 대해 조작별로 한 번씩 기록하여, 요청 body/query를 diff한다:

| 조작 | 주목할 점 |
|---|---|
| 리스트 첫 화면 | 페이지네이션 기본값 |
| 검색 | keyword, filters |
| 고급 필터 | optional 필드 |
| 신규/편집 | 완전한 entity |
| 일괄/내보내기 | `ids[]`, `exportType` |
| 정렬/페이지 넘김 | `sortField`, `order` |

`param_samples.json`을 산출: `[{ "path", "method", "action": "search", "body", "query", "headers" }]`

### J7. 신뢰도 규칙

| 신뢰도 | 조건 |
|---|---|
| **높음** | 정적 callsite + runtime ≥2 샘플 일치 |
| **중간** | 정적만 있거나, runtime이 1회만 있음 |
| **낮음** | 응답/오류로부터 역추론했으며, 2차 검증 미수행 |
| **대기 중** | 정적으로는 알려진 필드지만, UI/권한에 아직 도달하지 못함 |

### J8. 시나리오별 빠른 구성

| 시나리오 | 순서 |
|---|---|
| REST 리스트 페이지 | J1 조합 객체 → J6 네 번 diff → J3 rules |
| 신규/편집 폼 | J3 Form name → J4 submit 체인 → runtime 제출 + 일부러 비워두고 400 확인 |
| GraphQL | J2 variables 선언 → runtime에서 각 operation의 variables 기록 |
| 암호화된 body | J5 입력 파라미터 Hook → 암호화 전 필드가 실제 params |

### J9. api-recon과의 단계 매핑

| api-recon | 파라미터 recon |
|---|---|
| Phase 1 정적 분석 | J1 앵커 확장 윈도우 |
| Phase 2 A2 인터셉터 | 전역으로 주입되는 필드(tenantId, sign) |
| Phase 3 runtime | J6 트리거 매트릭스 + `param_samples.json` |
| Phase 4 권한 트리 | 모듈마다 폼이 다름 → 권한이 충분해야 전체 필드가 트리거됨 |
| Phase 5 병합 | `params_merged.json` + 신뢰도; 단일 샘플로 필수 여부를 단정하지 말 것 |

### J10. 트러블슈팅

| 현상 | 처리 |
|---|---|
| 정적 분석에는 필드명이 있지만 runtime에는 전혀 나타나지 않음 | "대기 중"으로 표시; 권한 트리 보완 / 고급 필터 클릭 / 연동 select의 각 option 시도 |
| 동일 path인데 다른 body 형태 | 정상 — `action`별로 나누어 기록하며, 스키마를 억지로 합치지 말 것 |
| stub 응답이 가짜지만 params를 보고 싶음 | **outbound 요청**의 body/headers를 볼 것, stub 응답으로부터 역추론하지 말 것 |
| 400 오류가 nested field를 가리킴 | 외곽 래핑 `data`/`bizData`/`variables`에 주의 |
| GraphQL에서 operation 이름만 보임 | `variables` JSON을 펼쳐볼 것; 정적 분석으로 `$var: Type` 찾기 |
