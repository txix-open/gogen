### v2.5.0
* миграция на `rand/v2` для ускорения генерации
* добавлена опция `ExternalCsvSource.DisableReadRandomMode`.
  * Если включена, то будут из источника будут использованы значения по порядку. Если они закончатся, то будут использован сначала
* добавлен флаг `pprofPort` для запуска `pprof`
### v2.4.0
* добавлена возможность указывать вероятность для `oneOf` полей
* добавлена поддержка указания кастомного алфавита для строковых полей
* добавлена возможность указывать в конфигурации значения `min` и `max` для `type = date`
* обновлена версия Go до `1.24`
* обновлены зависимости
### v2.2.1
* добавлена возможность указывать `csv` разделитель в конфигурации `entity`
### v2.2.0
* добавлена поддержка `oneOf` для полей в конфигурации
* добавлена поддержка источника данных для полей в виде внешнего файла в формате `csv`
### v2.0.0
* add `email`, `sequence` types to generate
* add option to format value after generation via `template`
* add option `asString` to stringify any generated value after generation
* move `reference` to `type` level for use `template` and `asString` options
