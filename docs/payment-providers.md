# Подключение новых платёжных сервисов

В **Админка → Интеграции** раскройте карточку сервиса, заполните реквизиты, включите интеграцию и сохраните. Ключи хранятся в зашифрованном виде в базе; добавлять их в `.env` не нужно.

После сохранения в карточке появится **Webhook URL**. Укажите его в кабинете платёжного сервиса, если это требуется в таблице. Входящие уведомления должны быть доступны по HTTPS. Сначала проверьте оплату в тестовом режиме своего сервиса: автотесты Link-Bot проверяют протокол и подписи на имитации API, реальные мерчантские ключи в них не используются.

| Сервис | Поля в Link-Bot | Настройка уведомлений | Официальная документация |
| --- | --- | --- | --- |
| anore.cc | Shop ID, API-ключ, Secret Key | Адрес передаётся при создании платежа как `callbackUrl`; Secret Key нужен для подписи уведомления | [anore.cc](https://anore.cc/docs) |
| MulenPay | Shop ID, API-ключ, секрет подписи из личного кабинета | Вставьте Webhook URL в настройку callback магазина | [MulenPay](https://docs.mulenpay.com/) |
| AuraPay | ID кассы, API-ключ, секретный ключ №2 | Адрес передаётся при создании счёта как `callback_url` | [AuraPay](https://docs.aurapay.tech/) |
| AntiloPay | Идентификатор проекта, Secret ID мерчанта, приватный ключ Base64, публичный ключ Callback Base64 | Вставьте Webhook URL в настройки уведомлений проекта | [AntiloPay API PDF](https://doc.antilopay.com/AntilopayAPI.pdf) |
| ParityPay | ID кассы, секретный ключ №1, секретный ключ №2 | Адрес передаётся при создании счёта как `callback_url` | [ParityPay v2](https://docs.paritypay.net/) |
| Tribute | API-ключ, Shop ID при нескольких магазинах | Вставьте Webhook URL в `callbackUrl` своего магазина | [Tribute Shop API](https://wiki.tribute.tg/for-shops/api/methods) |
| CloudPayments | Public ID и API Secret | Настройте уведомление **Pay**, метод **POST**, формат **CloudPayments**, URL из Link-Bot | [CloudPayments](https://developers.cloudpayments.ru/) |
| PayHot | Рабочий API-ключ кассы, секрет вебхука, способ оплаты | Вставьте Webhook URL в кабинете PayHot и выберите `payment.succeeded` | [API](https://docs.pay.hot/overview) · [Вебхуки](https://docs.pay.hot/webhooks) |
| Datagio | ID магазина, публичный API-ключ, секрет API, секрет вебхука | Вставьте Webhook URL и тот же секрет вебхука в настройки магазина | [Datagio Finance](https://wiki.datagio.finance/api/payments-create/) · [Вебхуки](https://wiki.datagio.finance/api/webhooks/) |
| Kassa AI | ID магазина, API-ключ, секретное слово №2, способ оплаты | Вставьте Webhook URL в кассу; адрес также передаётся как `notification_url` | [API `api.fk.life`](https://docs.freekassa.net/) |

Datagio использует серверные ключи `pk_live_...` и `sk_live_...`, эндпоинт `/merchant/v1/payment-links`. Магазин должен пройти модерацию. Укажите `webhook_secret` в магазине: Link-Bot принимает только подписанные уведомления `payment.succeeded` со статусом `paid`. Сумма проверяется по цене товара `amount`, а не по зачислению после комиссии `merchant_credit`.

Kassa AI использует отдельные реквизиты своей кассы и API `https://api.fk.life/v1/orders/create`: `44` — СБП, `36` — карты РФ, `43` — SberPay. Mini App передаёт IP покупателя; для оплаты непосредственно из Telegram-бота заполните поле «IP для платежей из бота». Email берётся из профиля/настроек или формируется как `TelegramID@telegram.org`. Задайте URL уведомлений в кассе; для переопределения `notification_url`, `success_url` и `failure_url` через API может потребоваться включение этой возможности поддержкой сервиса.

AntiloPay использует два разных ключа: приватный ключ мерчанта для исходящих запросов и публичный ключ проекта для Callback. Вставляйте Base64-значение, выданное кабинетом, без заголовков PEM. Приватный ключ должен быть RSA PKCS8, публичный — RSA SubjectPublicKeyInfo. Email покупателя берётся из подтверждённого Google-профиля или запрашивается в Mini App перед оплатой. IP передаётся из запроса Mini App.

Tribute подключается через **Shop API**, с разовыми заказами `period=onetime`. Нужен магазин, принимающий обычные платежи: магазин только для Stars не возвращает браузерную ссылку. Прежняя интеграция Tribute для подписок Telegram продолжает использовать свой отдельный тип платежа.

Описание заказа включает срок, устройства и трафик из тарифа/пакета. Успешное уведомление проходит проверку подписи, номера заказа, суммы и валюты. Для MulenPay, где подпись callback не описана, Link-Bot дополнительно запрашивает реальный статус платежа через авторизованный API. Повторный вебхук не начисляет покупку повторно.

## Сервисы, для которых нужна документация

В каталоге присутствует [Paycore](https://paycore.pw/), но включить его пока нельзя: интеграция отложена до получения документации. У других сервисов с таким названием API может отличаться.

Для продолжения нужны документация от поддержки сервиса или доступная ссылка на неё: создание счёта, авторизация, формат уведомления, подпись/проверка статуса и ответ на вебхук. До реализации такие сервисы не показываются покупателю среди способов оплаты.

## PayHot

В **Админке → Интеграции → PayHot** укажите рабочий API-ключ кассы (`phk_v2_…`), выданный PayHot **Webhook signing secret** и выберите способ оплаты. Ключу нужно право `merchant.payments.create`; выбранный способ и **RUB** должны быть подключены к кассе. Скопируйте показанный Webhook URL в раздел **Webhook** проекта PayHot и выберите `payment.succeeded`, затем включите и сохраните интеграцию.

Доступны СБП, карты, SberPay, криптовалюта, Apple Pay и Google Pay — в зависимости от настроек кассы. Email при необходимости покупатель вводит на странице оплаты. Тестовые ключи и события Sandbox не зачисляют реальные платежи.

Используется API v2, суммы в копейках и постоянный ключ идемпотентности для заказа. Подпись HMAC-SHA256 проверяется по исходному телу с допуском времени 5 минут. Оплата подтверждается только по `payment.succeeded` с полной списанной суммой, совпавшими заказом, ID платежа и валютой. Повторные уведомления не зачисляются повторно. При смене секрета обновите его в Link-Bot перед включением нового webhook endpoint.

## Официальные логотипы

Файлы `internal/miniapp/static/assets/payment-*.png` имеют размер 512×512. Логотипы скачаны с сайтов сервисов; SVG преобразованы в PNG, для некоторых выделен фирменный знак из полного логотипа. Для тёмных надписей сохранён белый фон. Новые логотипы не рисовались и не генерировались.

| PNG | Официальный исходник |
| --- | --- |
| `payment-datagio.png` | [PNG 512×512](https://datagio.finance/android-chrome-512x512.png) |
| `payment-paycore.png` | [SVG](https://paycore.pw/img/pc-logo-text-bottom.214db149.svg) |
| `payment-mulenpay.png` | [SVG](https://mulenpay.com/favicon.svg) |
| `payment-anore.png` | [SVG](https://anore.cc/images/anore-logo.svg) |
| `payment-aurapay.png` | [SVG](https://aurapay.tech/images/logo.svg) |
| `payment-antilopay.png` | [SVG с сайта AntiloPay](https://static.tildacdn.com/tild6661-3532-4138-b334-306134636237/antilopay_logo.svg) |
| `payment-paritypay.png` | [PNG](https://paritypay.net/upload/iblock/3ef/3oxj3264ncnfelp3v31hitg7lp9rsrda/logo.png) |
| `payment-tribute.png` | [Логотип в официальной документации Tribute](https://wiki.tribute.tg/) |
| `payment-kassaai.png` | [SVG](https://kassa.ai/assets/images/logo.svg) |
| `payment-cloudpayments.png` | [SVG с официального сайта CloudPayments](https://cdn.t-static.ru/static/pages/files/55eadcf3-411d-41f4-96ba-e893f9a4ca17.svg) |

Логотип PayHot: [официальная иконка кабинета](https://app.pay.hot/favicon-payhot.png), файл `payment-payhot.png`.
